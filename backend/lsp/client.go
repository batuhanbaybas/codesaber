package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// methodNotFound is the LSP error code returned when the server sends a
	// request we have no handler for (MVP: none are expected).
	methodNotFound int64 = -32601
	// closeWaitTimeout bounds waiting for the child process to exit after
	// shutdown/exit before killing it.
	closeWaitTimeout = 10 * time.Second
)

// OnDiagnostics receives textDocument/publishDiagnostics notifications,
// invoked on the reader goroutine; handlers must not block.
type OnDiagnostics func(uri string, diags []Diagnostic)

// Client is a JSON-RPC LSP connection to a language-server subprocess,
// with Content-Length framing in both directions. Safe for concurrent use.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	reqsMu sync.Mutex
	reqs   map[uint64]chan wireMessage

	writeMu sync.Mutex

	onDiagMu sync.Mutex
	onDiag   OnDiagnostics

	nextID atomic.Uint64

	closedMu sync.Mutex
	closed   bool
}

// newClientFromPipes wires a Client onto arbitrary pipes; used by tests to
// drive the protocol machinery without a subprocess. The caller must start
// reading (the constructor starts the read goroutine immediately).
func newClientFromPipes(stdout io.Reader, stdin io.WriteCloser, onDiag OnDiagnostics) *Client {
	c := &Client{
		stdin:  stdin,
		stdout: bufio.NewReaderSize(stdout, 64*1024),
		reqs:   make(map[uint64]chan wireMessage),
		onDiag: onDiag,
	}
	go c.readLoop()
	return c
}

// Start spawns binaryPath as a language-server subprocess rooted at rootURI,
// performs the initialize/initialized handshake and returns the client.
func Start(binaryPath, rootURI string) (*Client, error) {
	cmd := exec.Command(binaryPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp: stdout pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp: stdin pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lsp: spawn %s: %w", binaryPath, err)
	}
	c := newClientFromPipes(stdout, stdin, nil)
	c.cmd = cmd

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	initParams := map[string]any{
		"processId": os.Getpid(),
		"rootUri":   rootURI,
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"hover":           map[string]any{"contentFormat": []string{"markdown"}},
				"definition":      map[string]any{"linkSupport": false},
				"documentSymbol":  map[string]any{},
				"synchronization": map[string]any{"didSave": true},
			},
		},
		"initializationOptions": map[string]any{},
	}
	if _, err := c.rawRequest(ctx, "initialize", "initialize", initParams); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("lsp: initialize: %w", err)
	}
	if err := c.NotifySend("initialized", map[string]any{}); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("lsp: initialized notify: %w", err)
	}
	return c, nil
}

// SetDiagnosticsHandler registers (or clears with nil) the publishDiagnostics
// callback. Concurrent with the read loop via mutex.
func (c *Client) SetDiagnosticsHandler(fn OnDiagnostics) {
	c.onDiagMu.Lock()
	c.onDiag = fn
	c.onDiagMu.Unlock()
}

// NotifySend writes a notification frame (no id).
func (c *Client) NotifySend(method string, params any) error {
	return c.write(notification(method, params))
}

// rawRequest sends a request and returns the raw result, so callers can
// branch-decode varying result shapes. name is used in error messages.
func (c *Client) rawRequest(ctx context.Context, name, method string, params any) (json.RawMessage, error) {
	c.closedMu.Lock()
	closed := c.closed
	c.closedMu.Unlock()
	if closed {
		return nil, errors.New("lsp: client closed")
	}
	return c.sendRequest(ctx, name, method, params)
}

// sendRequest performs the request plumbing without consulting the closed
// flag; Close uses it so the shutdown sequence can bypass its own flag.
func (c *Client) sendRequest(ctx context.Context, name, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan wireMessage, 1)
	c.reqsMu.Lock()
	c.reqs[id] = ch
	c.reqsMu.Unlock()

	if err := c.write(request(id, method, params)); err != nil {
		c.reqsMu.Lock()
		delete(c.reqs, id)
		c.reqsMu.Unlock()
		return nil, fmt.Errorf("lsp: write %s request: %w", name, err)
	}

	select {
	case msg := <-ch:
		if msg.Error != nil {
			return nil, fmt.Errorf("lsp: %s: rpc error %d: %s", name, msg.Error.Code, msg.Error.Message)
		}
		return msg.Result, nil
	case <-ctx.Done():
		c.reqsMu.Lock()
		delete(c.reqs, id)
		c.reqsMu.Unlock()
		return nil, fmt.Errorf("lsp: %s: %w", name, ctx.Err())
	}
}

func (c *Client) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	return c.rawRequest(ctx, method, method, params)
}

func (c *Client) write(m message) error {
	b, err := encodeMessage(m)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err = c.stdin.Write(b)
	return err
}

func (c *Client) readLoop() {
	reader := &byteReader{r: c.stdout}
	for {
		body, err := readMessage(reader)
		if os.Getenv("LSP_DEBUG") != "" {
			fmt.Printf("readLoop frame: %q err=%v\n", body, err)
		}
		if err != nil {
			c.failAllPending(fmt.Errorf("lsp: stream error: %w", err))
			return
		}
		msg, err := decodeMessage(body)
		if err != nil {
			c.failAllPending(err)
			return
		}
		switch {
		case msg.isResp():
			var id uint64
			if err := json.Unmarshal(msg.ID, &id); err != nil {
				continue
			}
			c.reqsMu.Lock()
			ch, ok := c.reqs[id]
			if ok {
				delete(c.reqs, id)
			}
			c.reqsMu.Unlock()
			if ok {
				ch <- msg
			}
		case msg.isSrvReq():
			// MVP: no server→client requests are supported; answer
			// method-not-found so the server is not left waiting.
			var id any
			_ = json.Unmarshal(msg.ID, &id)
			_ = c.write(message{
				Jsonrpc: "2.0",
				ID:      id,
				Error:   &rpcError{Code: methodNotFound, Message: "lsp: no handler for " + msg.Method},
			})
		case msg.Method == "textDocument/publishDiagnostics":
			var params struct {
				URI         string       `json:"uri"`
				Diagnostics []Diagnostic `json:"diagnostics"`
			}
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				continue
			}
			c.onDiagMu.Lock()
			fn := c.onDiag
			c.onDiagMu.Unlock()
			if fn != nil {
				fn(params.URI, params.Diagnostics)
			}
		}
	}
}

func (c *Client) failAllPending(err error) {
	c.reqsMu.Lock()
	for id, ch := range c.reqs {
		select {
		case ch <- wireMessage{Error: &struct {
			Code    int64  `json:"code"`
			Message string `json:"message"`
		}{Code: -32000, Message: err.Error()}}:
		default:
		}
		delete(c.reqs, id)
	}
	c.reqsMu.Unlock()
}

// DidOpen sends textDocument/didOpen (full text).
func (c *Client) DidOpen(uri string, version int32, text string) error {
	return c.NotifySend("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{"uri": uri, "languageId": "go", "version": version, "text": text},
	})
}

// DidChange sends a full document sync change (versioned, single {text} entry).
func (c *Client) DidChange(uri string, version int32, text string) error {
	return c.NotifySend("textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": version},
		"contentChanges": []map[string]any{{"text": text}},
	})
}

// DidSave sends textDocument/didSave.
func (c *Client) DidSave(uri, text string) error {
	params := map[string]any{"textDocument": map[string]any{"uri": uri}}
	if text != "" {
		params["text"] = text
	}
	return c.NotifySend("textDocument/didSave", params)
}

// DidClose sends textDocument/didClose.
func (c *Client) DidClose(uri string) error {
	return c.NotifySend("textDocument/didClose", map[string]any{
		"textDocument": map[string]any{"uri": uri},
	})
}

// Definition resolves textDocument/definition. Handles null (no results),
// single Location, and array results. LocationLink arrays are decoded into
// their targetURI/targetSelection fields via best effort.
func (c *Client) Definition(ctx context.Context, uri string, line, char int32) ([]Location, error) {
	raw, err := c.request(ctx, "textDocument/definition", positionParams(uri, line, char))
	if err != nil {
		return nil, err
	}
	trimmed := trimSpaceJSON(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}
	var loc Location
	if err := json.Unmarshal(trimmed, &loc); err == nil && loc.URI != "" {
		return []Location{loc}, nil
	}
	var locs []Location
	if err := json.Unmarshal(trimmed, &locs); err == nil {
		return locs, nil
	}
	// LocationLink[]: map to Locations via targetUri/targetSelection.
	var links []struct {
		TargetURI   string `json:"targetUri"`
		TargetRange Range  `json:"targetSelection"`
	}
	if err := json.Unmarshal(trimmed, &links); err != nil {
		return nil, fmt.Errorf("lsp: definition: unexpected result shape %s", truncated(raw))
	}
	out := make([]Location, 0, len(links))
	for _, l := range links {
		out = append(out, Location{URI: l.TargetURI, Range: l.TargetRange})
	}
	return out, nil
}

// Hover resolves textDocument/hover; a null result yields nil.
func (c *Client) Hover(ctx context.Context, uri string, line, char int32) (*Hover, error) {
	raw, err := c.request(ctx, "textDocument/hover", positionParams(uri, line, char))
	if err != nil {
		return nil, err
	}
	trimmed := trimSpaceJSON(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}
	var h Hover
	if err := json.Unmarshal(trimmed, &h); err != nil {
		return nil, fmt.Errorf("lsp: hover: %w", err)
	}
	return &h, nil
}

// DocumentSymbols resolves textDocument/documentSymbol, tolerating both the
// hierarchical DocumentSymbol shape and the flat SymbolInformation shape
// (flat entries are mapped onto DocumentSymbol with location ranges).
func (c *Client) DocumentSymbols(ctx context.Context, uri string) ([]DocumentSymbol, error) {
	raw, err := c.request(ctx, "textDocument/documentSymbol", map[string]any{
		"textDocument": map[string]any{"uri": uri},
	})
	if err != nil {
		return nil, err
	}
	trimmed := trimSpaceJSON(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}
	var syms []DocumentSymbol
	if err := json.Unmarshal(trimmed, &syms); err == nil {
		return syms, nil
	}
	var infos []SymbolInformation
	if err := json.Unmarshal(trimmed, &infos); err != nil {
		return nil, fmt.Errorf("lsp: documentSymbol: unexpected result shape %s", truncated(raw))
	}
	out := make([]DocumentSymbol, 0, len(infos))
	for _, in := range infos {
		out = append(out, DocumentSymbol{
			Name:           in.Name,
			Kind:           in.Kind,
			Range:          in.Location.Range,
			SelectionRange: in.Location.Range,
		})
	}
	return out, nil
}

func positionParams(uri string, line, char int32) map[string]any {
	return map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     Position{Line: line, Character: char},
	}
}

// Close performs the graceful LSP shutdown sequence: shutdown request,
// exit notification, then waits up to closeWaitTimeout for the process to
// exit before killing it.
func (c *Client) Close() error {
	c.closedMu.Lock()
	if c.closed {
		c.closedMu.Unlock()
		return nil
	}
	c.closed = true
	c.closedMu.Unlock()

	// Issue the shutdown request and exit notification back-to-back without
	// waiting for the shutdown response; servers process stdin sequentially
	// so the full sequence is observed, then EOF tears down the process.
	id := c.nextID.Add(1)
	_ = c.write(request(id, "shutdown", nil))
	_ = c.write(notification("exit", nil))
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd == nil || c.cmd.Process == nil {
		return nil
	}
	done := make(chan struct{})
	go func() { _ = c.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(closeWaitTimeout):
		_ = c.cmd.Process.Kill()
		<-done
	}
	return nil
}

func trimSpaceJSON(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '\r' || b[0] == '\n') {
		b = b[1:]
	}
	for len(b) > 0 {
		switch b[len(b)-1] {
		case ' ', '\t', '\r', '\n':
			b = b[:len(b)-1]
		default:
			return b
		}
	}
	return b
}

func truncated(b json.RawMessage) string {
	s := string(b)
	if len(s) > 120 {
		s = s[:120] + "…"
	}
	return s
}

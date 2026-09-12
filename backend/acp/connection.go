package acp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// RPC error codes used when the client responds to agent->client requests.
const (
	RPCMethodNotFound  = -32601
	RPCInternalError   = -32000
	closeWaitTimeout   = 2 * time.Second
	stderrRingCapacity = 4096
)

// Error implements the error interface so RPCError can be unwrapped from
// handler errors (errors.As) when customizing the response code.
func (e *RPCError) Error() string {
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

// ClientHandlers carries the client-side callbacks invoked when the agent
// sends a request to the client (fs access, permission prompts). Nil handlers
// are answered with a method-not-found error.
type ClientHandlers struct {
	ReadTextFile  func(sessionID, path string) (string, error)
	WriteTextFile func(sessionID, path, content string) error
	// RequestPermission receives raw session/request_permission params and
	// returns the option id the user picked.
	RequestPermission func(params map[string]any) (any, error)
}

// Conn is a JSON-RPC stdio connection to an ACP agent child process.
// It is safe for concurrent use.
type Conn struct {
	cmd *exec.Cmd

	// child stdout (we read), child stdin (we write)
	stdout io.Reader
	stdin  io.WriteCloser

	reqsMu sync.Mutex
	reqs   map[uint64]chan Frame

	nextID atomic.Uint64

	handlers ClientHandlers

	// Notify receives frames from the agent that are not responses to our
	// requests and not agent->client requests (i.e. notifications), for the
	// session client layer to consume.
	Notify chan Frame

	writeMu sync.Mutex

	stderrMu  sync.Mutex
	stderrBuf []byte

	closed atomic.Bool
}

// newConnFromPipes wires a Conn onto arbitrary pipes (used by tests to drive
// the protocol machinery without spawning a real child process).
func newConnFromPipes(stdout io.Reader, stdin io.WriteCloser, handlers ClientHandlers) *Conn {
	c := &Conn{
		stdout:   stdout,
		stdin:    stdin,
		reqs:     make(map[uint64]chan Frame),
		handlers: handlers,
		Notify:   make(chan Frame, 64),
	}
	go c.readLoop()
	return c
}

// Spawn launches an ACP agent command and returns a connected Conn.
func Spawn(profile Info, handlers ClientHandlers) (*Conn, error) {
	if len(profile.Command) == 0 {
		return nil, errors.New("acp: empty command for profile " + profile.Name)
	}
	cmd := exec.Command(profile.Command[0], profile.Command[1:]...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: stdout pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: stdin pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("acp: spawn %s: %w", profile.Name, err)
	}
	c := newConnFromPipes(stdout, stdin, handlers)
	c.cmd = cmd
	go c.drainStderr(stderr)
	return c, nil
}

// Request sends a client->agent request and blocks until the matching
// response, an RPC error, or ctx cancellation.
func (c *Conn) Request(ctx context.Context, method string, params any) (any, error) {
	if c.closed.Load() {
		return nil, errors.New("acp: connection closed")
	}
	id := c.nextID.Add(1)
	ch := make(chan Frame, 1)
	c.reqsMu.Lock()
	c.reqs[id] = ch
	c.reqsMu.Unlock()

	if err := c.writeLine(NewRequest(id, method, params)); err != nil {
		c.reqsMu.Lock()
		delete(c.reqs, id)
		c.reqsMu.Unlock()
		return nil, fmt.Errorf("acp: write request: %w", err)
	}

	select {
	case f := <-ch:
		if f.Error != nil {
			return nil, fmt.Errorf("acp: %s: %w", method, f.Error)
		}
		return f.Result, nil
	case <-ctx.Done():
		c.reqsMu.Lock()
		delete(c.reqs, id)
		c.reqsMu.Unlock()
		return nil, fmt.Errorf("acp: %s: %w", method, ctx.Err())
	}
}

// NotifySend writes a notification line (no id, no response expected).
func (c *Conn) NotifySend(method string, params any) error {
	return c.writeLine(NewNotify(method, params))
}

// Close performs a graceful shutdown: close the agent's stdin, wait briefly
// for exit, then kill.
func (c *Conn) Close() error {
	if c.closed.Swap(true) {
		return nil
	}
	if c.stdin != nil {
		c.stdin.Close()
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

// Stderr returns the most recent stderr output from the agent (ring buffer,
// capped at 4KB) for error surfacing.
func (c *Conn) Stderr() string {
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()
	return string(c.stderrBuf)
}

func (c *Conn) writeLine(f Frame) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err := c.stdin.Write(f.MarshalLine())
	return err
}

func (c *Conn) readLoop() {
	sc := bufio.NewScanner(c.stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		f, err := DecodeLine(sc.Bytes())
		if err != nil {
			continue // tolerate unparseable lines
		}
		c.dispatch(f)
	}
	// Stream ended: fail all pending requests.
	c.failAllPending(errors.New("acp: agent stream closed"))
}

func (c *Conn) failAllPending(err error) {
	c.reqsMu.Lock()
	for id, ch := range c.reqs {
		select {
		case ch <- Frame{Error: &RPCError{Code: RPCInternalError, Message: err.Error()}}:
		default:
		}
		delete(c.reqs, id)
	}
	c.reqsMu.Unlock()
}

func (c *Conn) dispatch(f Frame) {
	switch {
	case f.Method == "" && f.ID != nil:
		// response to one of our requests
		if id, ok := numericID(f.ID); ok {
			c.reqsMu.Lock()
			ch, exists := c.reqs[id]
			if exists {
				delete(c.reqs, id)
			}
			c.reqsMu.Unlock()
			if exists {
				ch <- f
			}
		}
	case f.Method != "" && f.ID != nil:
		// agent->client request: invoke handler, write response back
		c.handleAgentRequest(f)
	case f.Method != "":
		// notification for the session client layer
		c.Notify <- f
	}
}

func (c *Conn) handleAgentRequest(f Frame) {
	result, rpcErr := c.invokeHandler(f.Method, f.Params)
	var resp Frame
	if rpcErr != nil {
		var codeErr *RPCError
		code := RPCInternalError
		if errors.As(rpcErr, &codeErr) {
			code = codeErr.Code
		}
		resp = NewErrorResponse(f.ID, code, rpcErr.Error())
	} else {
		resp = NewResponse(f.ID, result)
	}
	// Agent stdin gone; nothing to do, read loop will observe EOF.
	_ = c.writeLine(resp)
}

var errUnsupportedClientMethod = errors.New("acp: unsupported client method")

func (c *Conn) invokeHandler(method string, params any) (any, error) {
	m, _ := params.(map[string]any)
	sessionID, _ := m["sessionId"].(string)
	path, _ := m["path"].(string)
	switch method {
	case MethodFsReadTextFile:
		if c.handlers.ReadTextFile == nil {
			return nil, errUnsupportedClientMethod
		}
		content, err := c.handlers.ReadTextFile(sessionID, path)
		if err != nil {
			return nil, err
		}
		return map[string]any{"content": content}, nil
	case MethodFsWriteTextFile:
		if c.handlers.WriteTextFile == nil {
			return nil, errUnsupportedClientMethod
		}
		content, _ := m["content"].(string)
		if err := c.handlers.WriteTextFile(sessionID, path, content); err != nil {
			return nil, err
		}
		return map[string]any{}, nil
	case MethodRequestPermission:
		if c.handlers.RequestPermission == nil {
			return nil, errUnsupportedClientMethod
		}
		optionID, err := c.handlers.RequestPermission(m)
		if err != nil {
			return nil, err
		}
		return map[string]any{"optionId": optionID}, nil
	default:
		return nil, &RPCError{Code: RPCMethodNotFound, Message: "acp: no handler for " + method}
	}
}

func (c *Conn) drainStderr(r io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			c.appendStderr(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func (c *Conn) appendStderr(b []byte) {
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()
	c.stderrBuf = append(c.stderrBuf, b...)
	if len(c.stderrBuf) > stderrRingCapacity {
		c.stderrBuf = c.stderrBuf[len(c.stderrBuf)-stderrRingCapacity:]
	}
}

func numericID(id any) (uint64, bool) {
	switch v := id.(type) {
	case float64:
		return uint64(v), true
	case uint64:
		return v, true
	case int:
		return uint64(v), true
	default:
		return 0, false
	}
}

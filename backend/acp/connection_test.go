package acp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// harnessConn builds a Conn wired to two os.Pipe pairs, standing in for a child
// agent process: writes to agentIn are read by the Conn (child stdout), writes
// by the Conn to clientOut are read by the test (child stdin).
type harnessConn struct {
	conn      *Conn
	agentIn   *os.File // write side the fake agent "stdout" feeds
	clientOut *os.File // read side of what the Conn writes (child stdin)
}

func newHarness(t *testing.T, handlers ClientHandlers) *harnessConn {
	t.Helper()
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(stdout): %v", err)
	}
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(stdin): %v", err)
	}
	t.Cleanup(func() {
		stdoutW.Close()
		stdinR.Close()
	})
	conn := newConnFromPipes(stdoutR, stdinW, handlers)
	t.Cleanup(func() { conn.Close() })
	return &harnessConn{conn: conn, agentIn: stdoutW, clientOut: stdinR}
}

// readLine reads one newline-terminated JSON-RPC line written by the Conn.
func (h *harnessConn) readLine(t *testing.T) string {
	t.Helper()
	buf := make([]byte, 0, 4096)
	b := make([]byte, 1)
	for {
		n, err := h.clientOut.Read(b)
		if n == 1 {
			if b[0] == '\n' {
				return string(buf)
			}
			buf = append(buf, b[0])
			continue
		}
		if err != nil {
			t.Fatalf("readLine: %v (buf=%q)", err, buf)
		}
	}
}

// serveAgent is a minimal fake agent loop: reads request lines written by the
// Conn (client stdin) and writes response lines to its own stdout.
func (h *harnessConn) serveAgent(t *testing.T, respond func(id any, method string) string) {
	t.Helper()
	go func() {
		sc := bufio.NewScanner(h.clientOut)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			f, err := DecodeLine(sc.Bytes())
			if err != nil {
				continue
			}
			if f.ID == nil || f.Method == "" {
				continue
			}
			h.agentIn.Write([]byte(respond(f.ID, f.Method) + "\n"))
		}
	}()
}

func TestConnRequestResponseRoundtrip(t *testing.T) {
	h := newHarness(t, ClientHandlers{})
	h.serveAgent(t, func(id any, method string) string {
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":%v,"result":{"ok":true}}`, id)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	res, err := h.conn.Request(ctx, "initialize", map[string]any{"protocolVersion": 1})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok || m["ok"] != true {
		t.Fatalf("res = %#v, want {ok:true}", res)
	}
}

func TestConnNotificationDelivery(t *testing.T) {
	h := newHarness(t, ClientHandlers{})

	line := NewNotify(MethodSessionUpdate, map[string]any{"sessionUpdate": UpdateAgentMessageChunk}).MarshalLine()
	if _, err := h.agentIn.Write(line); err != nil {
		t.Fatalf("write notify: %v", err)
	}

	select {
	case f := <-h.conn.Notify:
		if f.Method != MethodSessionUpdate {
			t.Fatalf("method = %q, want %q", f.Method, MethodSessionUpdate)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for notification")
	}
}

func TestConnAgentToClientRequestHandled(t *testing.T) {
	handlers := ClientHandlers{
		ReadTextFile: func(sessionID, path string) (string, error) {
			if sessionID != "test-1" || path != "/tmp/f.txt" {
				t.Errorf("handler args = (%q, %q)", sessionID, path)
			}
			return "file content", nil
		},
	}
	h := newHarness(t, handlers)

	line := NewRequest(9, MethodFsReadTextFile, map[string]any{"sessionId": "test-1", "path": "/tmp/f.txt"}).MarshalLine()
	if _, err := h.agentIn.Write(line); err != nil {
		t.Fatalf("write request: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	h.clientOut.SetReadDeadline(deadline)
	raw := h.readLine(t)
	h.clientOut.SetReadDeadline(time.Time{})

	f, err := DecodeLine([]byte(raw))
	if err != nil {
		t.Fatalf("decode response: %v (%s)", err, raw)
	}
	if f.Method != "" {
		t.Fatalf("response must not carry method: %s", raw)
	}
	if f.ID == nil || fmt.Sprintf("%v", f.ID) != "9" {
		t.Fatalf("id = %v, want 9", f.ID)
	}
	if f.Result == nil {
		t.Fatalf("missing result: %s", raw)
	}
	m, _ := f.Result.(map[string]any)
	if m == nil || m["content"] != "file content" {
		t.Fatalf("result = %#v, want {content:\"file content\"}", f.Result)
	}
}

func TestConnAgentToClientRequestErrorResponded(t *testing.T) {
	handlers := ClientHandlers{
		ReadTextFile: func(sessionID, path string) (string, error) {
			return "", errors.New("no such file")
		},
	}
	h := newHarness(t, handlers)

	line := NewRequest(11, MethodFsReadTextFile, map[string]any{"sessionId": "s", "path": "/x"}).MarshalLine()
	if _, err := h.agentIn.Write(line); err != nil {
		t.Fatalf("write request: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	h.clientOut.SetReadDeadline(deadline)
	raw := h.readLine(t)
	h.clientOut.SetReadDeadline(time.Time{})

	f, err := DecodeLine([]byte(raw))
	if err != nil {
		t.Fatalf("decode: %v (%s)", err, raw)
	}
	if f.Error == nil {
		t.Fatalf("want error response, got: %s", raw)
	}
	if f.Error.Code != RPCInternalError {
		t.Fatalf("code = %d, want %d", f.Error.Code, RPCInternalError)
	}
}

func TestConnRPCErrorSurfacesAsGoError(t *testing.T) {
	h := newHarness(t, ClientHandlers{})
	h.serveAgent(t, func(id any, method string) string {
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":%v,"error":{"code":-32000,"message":"boom"}}`, id)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := h.conn.Request(ctx, "session/prompt", map[string]any{})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want it to contain rpc message", err)
	}
}

func TestConnConcurrentRequests(t *testing.T) {
	h := newHarness(t, ClientHandlers{})
	h.serveAgent(t, func(id any, method string) string {
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":%v,"result":{"id":%v}}`, id, id)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := h.conn.Request(ctx, "ping", nil)
			if err != nil {
				errs <- err
				return
			}
			m := res.(map[string]any)
			if m["id"] == nil {
				errs <- errors.New("missing id in result")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent request: %v", err)
	}
}

func TestConnRequestContextTimeout(t *testing.T) {
	h := newHarness(t, ClientHandlers{}) // no responder: request hangs

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := h.conn.Request(ctx, "initialize", nil)
	if err == nil {
		t.Fatal("want timeout error, got nil")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("took %v, ctx should cut it short", elapsed)
	}
}

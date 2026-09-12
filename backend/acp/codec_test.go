package acp

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func TestNewRequestMarshalLine(t *testing.T) {
	f := NewRequest(1, "session/prompt", map[string]any{"sessionId": "s1"})
	line := f.MarshalLine()
	if !strings.HasSuffix(string(line), "\n") {
		t.Fatalf("line must end with newline: %q", line)
	}
	if strings.Contains(string(line), "\n") && strings.Count(string(line), "\n") != 1 {
		t.Fatalf("line must contain no embedded newlines: %q", line)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(line))), &m); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if m["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v, want 2.0", m["jsonrpc"])
	}
	if m["method"] != "session/prompt" {
		t.Errorf("method = %v", m["method"])
	}
}

func TestNewNotifyOmitsID(t *testing.T) {
	f := NewNotify("session/cancel", map[string]any{"sessionId": "s1"})
	line := string(f.MarshalLine())
	if strings.Contains(line, `"id"`) {
		t.Errorf("notification must not contain id: %s", line)
	}
	if !strings.Contains(line, `"method":"session/cancel"`) {
		t.Errorf("missing method: %s", line)
	}
}

func TestNewResponseMarshal(t *testing.T) {
	f := NewResponse(7, map[string]any{"stopReason": "end_turn"})
	line := string(f.MarshalLine())
	if !strings.Contains(line, `"id":7`) {
		t.Errorf("missing id: %s", line)
	}
	if strings.Contains(line, `"method"`) || strings.Contains(line, `"error"`) {
		t.Errorf("response must not contain method/error: %s", line)
	}
}

func TestNewErrorResponseMarshal(t *testing.T) {
	f := NewErrorResponse(3, -32601, "method not found")
	line := string(f.MarshalLine())
	if !strings.Contains(line, `"error":{"code":-32601,"message":"method not found"}`) {
		t.Errorf("bad error object: %s", line)
	}
}

func TestDecodeLineClassifies(t *testing.T) {
	cases := []struct {
		name  string
		line  string
		check func(t *testing.T, f Frame, err error)
	}{
		{
			name: "request",
			line: `{"jsonrpc":"2.0","id":1,"method":"session/prompt","params":{"sessionId":"s"}}`,
			check: func(t *testing.T, f Frame, err error) {
				if err != nil || f.Method != "session/prompt" || f.ID == nil || f.Error != nil || f.Result != nil {
					t.Errorf("got %+v, %v", f, err)
				}
			},
		},
		{
			name: "request missing jsonrpc",
			line: `{"id":5,"method":"initialize","params":{"protocolVersion":1}}`,
			check: func(t *testing.T, f Frame, err error) {
				if err != nil || f.JSONRPC != "" || f.Method != "initialize" || f.ID == nil {
					t.Errorf("got %+v, %v", f, err)
				}
			},
		},
		{
			name: "string id request",
			line: `{"id":"a-1","method":"session/new"}`,
			check: func(t *testing.T, f Frame, err error) {
				if err != nil || f.Method != "session/new" || f.ID != "a-1" {
					t.Errorf("got %+v, %v", f, err)
				}
			},
		},
		{
			name: "notification",
			line: `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s"}}`,
			check: func(t *testing.T, f Frame, err error) {
				if err != nil || f.Method != "session/update" || f.ID != nil {
					t.Errorf("got %+v, %v", f, err)
				}
			},
		},
		{
			name: "error response",
			line: `{"jsonrpc":"2.0","id":2,"error":{"code":-32601,"message":"method not found"}}`,
			check: func(t *testing.T, f Frame, err error) {
				if err != nil || f.Error == nil || f.Error.Code != -32601 || f.ID == nil {
					t.Errorf("got %+v, %v", f, err)
				}
			},
		},
		{
			name: "result response",
			line: `{"jsonrpc":"2.0","id":9,"result":{"stopReason":"end_turn"}}`,
			check: func(t *testing.T, f Frame, err error) {
				if err != nil || f.Error != nil || f.Result == nil {
					t.Errorf("got %+v, %v", f, err)
				}
			},
		},
		{
			name: "invalid json",
			line: `{not json`,
			check: func(t *testing.T, f Frame, err error) {
				if err == nil {
					t.Errorf("want error for invalid json")
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := DecodeLine([]byte(tc.line))
			tc.check(t, f, err)
		})
	}
}

func TestRoundTrip(t *testing.T) {
	f := NewRequest("x1", "fs/read_text_file", map[string]any{"path": "/a", "sessionId": "s"})
	dec, err := DecodeLine(f.MarshalLine())
	if err != nil {
		t.Fatalf("roundtrip: %v", err)
	}
	if dec.Method != "fs/read_text_file" || dec.ID != "x1" {
		t.Errorf("roundtrip mismatch: %+v", dec)
	}
}

func TestNewIDUniqueConcurrent(t *testing.T) {
	const n = 200
	var wg sync.WaitGroup
	seen := make(map[uint64]struct{}, n)
	ids := make(chan uint64, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids <- NewID()
		}()
	}
	wg.Wait()
	close(ids)
	for id := range ids {
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id %d", id)
		}
		seen[id] = struct{}{}
	}
}

// Command lspfake is a minimal LSP server used by provider tests to
// exercise the real subprocess path (Start → requests → shutdown/exit)
// without gopls. Wire behavior: Content-Length framed JSON-RPC over stdio.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type frame struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Result  any             `json:"result,omitempty"`
	Params  any             `json:"params,omitempty"`
}

func reply(w io.Writer, id json.RawMessage, result any) {
	write(w, frame{Jsonrpc: "2.0", ID: id, Result: result})
}

func write(w io.Writer, f frame) {
	b, _ := json.Marshal(f)
	fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(b))
	_, _ = w.Write(b)
}

type req struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func main() {
	r := bufio.NewReader(os.Stdin)
	for {
		contentLength := -1
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			if line == "" {
				break
			}
			if strings.HasPrefix(strings.ToLower(line), "content-length") {
				fmt.Sscanf(line, "Content-Length: %d", &contentLength)
			}
		}
		if contentLength < 0 {
			return
		}
		body := make([]byte, contentLength)
		if _, err := io.ReadFull(r, body); err != nil {
			return
		}
		var rq req
		if json.Unmarshal(body, &rq) != nil {
			continue
		}
		switch rq.Method {
		case "initialize":
			reply(os.Stdout, rq.ID, map[string]any{"capabilities": map[string]any{}})
		case "textDocument/didOpen":
			var p struct {
				TextDocument struct {
					URI string `json:"uri"`
				} `json:"textDocument"`
			}
			_ = json.Unmarshal(rq.Params, &p)
			write(os.Stdout, frame{
				Jsonrpc: "2.0",
				Method:  "textDocument/publishDiagnostics",
				Params: map[string]any{
					"uri": p.TextDocument.URI,
					"diagnostics": []map[string]any{{
						"range": mkRange(2, 0, 2, 5),
						"severity": 1,
						"message": "fake diagnostic",
					}},
				},
			})
		case "textDocument/definition":
			reply(os.Stdout, rq.ID, []map[string]any{{
				"uri": "file:///x/other.go",
				"range": map[string]any{
					"start": map[string]any{"line": 4, "character": 2},
					"end":   map[string]any{"line": 4, "character": 9},
				},
			}})
		case "textDocument/hover":
			reply(os.Stdout, rq.ID, map[string]any{"contents": "hover doc"})
		case "textDocument/documentSymbol":
			reply(os.Stdout, rq.ID, []map[string]any{{
				"name":           "Main",
				"kind":           12,
				"range":          mkRange(0, 0, 10, 1),
				"selectionRange": mkRange(1, 5, 1, 9),
			}})
		case "shutdown":
			reply(os.Stdout, rq.ID, nil)
		case "exit":
			return
		}
	}
}

func mkRange(l1, c1, l2, c2 int32) map[string]any {
	return map[string]any{
		"start": map[string]any{"line": l1, "character": c1},
		"end":   map[string]any{"line": l2, "character": c2},
	}
}

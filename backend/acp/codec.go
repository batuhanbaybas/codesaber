package acp

import (
	"encoding/json"
	"errors"
	"sync/atomic"
)

// JSONRPCVersion identifies wire frames as JSON-RPC 2.0 (ACP stdio transport).
const JSONRPCVersion = "2.0"

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Frame is a single JSON-RPC message line serving both peer roles:
// client->agent requests, agent->client requests, notifications and responses.
type Frame struct {
	JSONRPC string    `json:"jsonrpc,omitempty"`
	ID      any       `json:"id,omitempty"`
	Method  string    `json:"method,omitempty"`
	Params  any       `json:"params,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

func NewRequest(id any, method string, params any) Frame {
	return Frame{JSONRPC: JSONRPCVersion, ID: id, Method: method, Params: params}
}

func NewNotify(method string, params any) Frame {
	return Frame{JSONRPC: JSONRPCVersion, Method: method, Params: params}
}

func NewResponse(id any, result any) Frame {
	return Frame{JSONRPC: JSONRPCVersion, ID: id, Result: result}
}

func NewErrorResponse(id any, code int, msg string) Frame {
	return Frame{JSONRPC: JSONRPCVersion, ID: id, Error: &RPCError{Code: code, Message: msg}}
}

// MarshalLine encodes f as one newline-delimited JSON-RPC line (ACP stdio transport).
func (f Frame) MarshalLine() []byte {
	b, err := json.Marshal(f)
	if err != nil {
		// Params/Result carried a non-marshalable value; degrade to an empty object
		// line so the stream stays parseable.
		b = []byte("{}")
	}
	return append(b, '\n')
}

// DecodeLine parses one JSON-RPC line tolerantly (jsonrpc key optional) and
// classifies it: request (method + id), notification (method, no id),
// error-response, or result-response.
func DecodeLine(line []byte) (Frame, error) {
	var f Frame
	if err := json.Unmarshal(line, &f); err != nil {
		return f, errors.New("acp: invalid JSON-RPC line: " + err.Error())
	}
	return f, nil
}

var idCounter atomic.Uint64

// NewID returns a process-unique numeric JSON-RPC request id.
func NewID() uint64 {
	return idCounter.Add(1)
}

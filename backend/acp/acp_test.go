package acp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCapabilitiesWireNames(t *testing.T) {
	caps := ClientCapabilities{
		FileSystem: FsCapability{ReadTextFile: true, WriteTextFile: true},
	}
	b, _ := json.Marshal(caps)
	got := string(b)
	if !strings.Contains(got, `"fs":{"readTextFile":true,"writeTextFile":true}`) {
		t.Errorf("bad fs caps wire: %s", got)
	}

	agentCaps := AgentCapabilities{LoadSession: true}
	b, _ = json.Marshal(agentCaps)
	if string(b) != `{"loadSession":true}` {
		t.Errorf("bad agent caps wire: %s", string(b))
	}
}

func TestFsCapabilityOmitsFalse(t *testing.T) {
	b, _ := json.Marshal(FsCapability{})
	if string(b) != "{}" {
		t.Errorf("false caps must be omitted, got %s", string(b))
	}
}

func TestProtocolVersionConstant(t *testing.T) {
	if ProtocolVersion != 1 {
		t.Errorf("ProtocolVersion = %d, want 1", ProtocolVersion)
	}
}

func TestPromptParamsWireShape(t *testing.T) {
	params, err := json.Marshal(NewPromptParams("sess-1", []PromptItem{
		{Type: BlockTypeText, Text: "hello"},
	}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(params, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["sessionId"] != "sess-1" {
		t.Errorf("sessionId = %v", m["sessionId"])
	}
	prompt, ok := m["prompt"].([]any)
	if !ok || len(prompt) != 1 {
		t.Fatalf("prompt = %v", m["prompt"])
	}
	item := prompt[0].(map[string]any)
	if item["type"] != "text" || item["text"] != "hello" {
		t.Errorf("prompt item = %v", item)
	}
}

func TestPromptResponseWireShape(t *testing.T) {
	b, _ := json.Marshal(PromptResponse{StopReason: StopReasonEndTurn})
	if string(b) != `{"stopReason":"end_turn"}` {
		t.Errorf("bad stopReason wire: %s", string(b))
	}
}

func TestSessionUpdateAgentMessageChunkWire(t *testing.T) {
	upd := SessionUpdate{
		SessionUpdate: UpdateAgentMessageChunk,
		Content:       &ContentBlock{Type: BlockTypeText, Text: "hi"},
		ID:            "sess-1",
	}
	b, _ := json.Marshal(upd)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["sessionUpdate"] != "agent_message_chunk" {
		t.Errorf("update discriminator = %v", m["sessionUpdate"])
	}
	if m["sessionId"] != "sess-1" {
		t.Errorf("sessionId = %v", m["sessionId"])
	}
	content, ok := m["content"].(map[string]any)
	if !ok || content["type"] != "text" || content["text"] != "hi" {
		t.Errorf("content = %v", m["content"])
	}
	if m["toolCallId"] != nil || m["toolCall"] != nil {
		t.Errorf("message chunk must not carry toolCall fields: %v", m)
	}
}

func TestSessionUpdateToolCallVariantWire(t *testing.T) {
	upd := SessionUpdate{
		SessionUpdate: UpdateToolCall,
		ID:            "sess-1",
		ToolCallID:    "call-1",
		Title:         "edit.go",
		Kind:          ToolKindEdit,
		Status:        ToolStatusPending,
	}
	b, _ := json.Marshal(upd)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["toolCallId"] != "call-1" || m["title"] != "edit.go" {
		t.Errorf("tool_call variant must be flat per spec: %v", m)
	}
	if m["kind"] != "edit" || m["status"] != "pending" {
		t.Errorf("kind/status = %v/%v", m["kind"], m["status"])
	}
}

func TestToolCallNestedForPermissionWire(t *testing.T) {
	tc := ToolCall{
		ID:     "call-1",
		Title:  "rm",
		Kind:   ToolKindDelete,
		Status: ToolStatusPending,
	}
	b, _ := json.Marshal(tc)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["id"] != "call-1" || m["title"] != "rm" || m["kind"] != "delete" {
		t.Errorf("nested toolCall wire = %v", m)
	}
}

func TestPermissionRequestWireShape(t *testing.T) {
	req, err := json.Marshal(NewPermissionParams("sess-1", []any{
		map[string]any{"optionId": "allow", "name": "Allow", "kind": "allow_once"},
	}, ToolCall{ID: "call-1", Title: "rm"}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(req, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["sessionId"] != "sess-1" || m["options"] == nil {
		t.Errorf("permission request = %v", m)
	}
	if _, ok := m["toolCall"]; !ok {
		t.Errorf("missing toolCall in permission request: %v", m)
	}
}

func TestDecodedNotifySkipsError(t *testing.T) {
	f := NewNotify("session/cancel", map[string]any{"sessionId": "s"})
	dec, err := DecodeLine(f.MarshalLine())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if dec.Error != nil || dec.Method != "session/cancel" {
		t.Errorf("got %+v", dec)
	}
}

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

// ContentItems shares the `content` wire key with Content; the array form is
// the tool_call/tool_call_update shape (v1 schema).
func TestSessionUpdateToolCallContentItemsWire(t *testing.T) {
	upd := SessionUpdate{
		SessionUpdate: UpdateToolCallUpdate,
		ID:            "sess-1",
		ToolCallID:    "call-1",
		Status:        ToolStatusCompleted,
		ContentItems: []ContentBlock{
			{Type: BlockTypeText, Text: "output line"},
		},
	}
	b, err := json.Marshal(upd)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	content, ok := m["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("content = %v, want one-element array", m["content"])
	}
	blk, _ := content[0].(map[string]any)
	if blk["type"] != "text" || blk["text"] != "output line" {
		t.Errorf("content block = %v", blk)
	}
	if m["toolCall"] != nil {
		t.Errorf("tool_call_update must not carry nested toolCall: %v", m)
	}

	// round-trip: array form decodes back into ContentItems
	var back SessionUpdate
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("roundtrip unmarshal: %v", err)
	}
	if len(back.ContentItems) != 1 || back.ContentItems[0].Text != "output line" {
		t.Fatalf("roundtrip ContentItems = %#v", back.ContentItems)
	}
	if back.Content != nil {
		t.Fatalf("roundtrip Content = %#v, want nil", back.Content)
	}
}

func TestSessionUpdateUnmarshalContentObjectForm(t *testing.T) {
	// decodeSessionUpdate unwraps the v1 nested {update:{...}} wrapper before
	// unmarshal, so decode the inner object here.
	raw := []byte(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"hi"}}`)
	var u SessionUpdate
	if err := json.Unmarshal(raw, &u); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if u.Content == nil || u.Content.Text != "hi" {
		t.Fatalf("Content = %#v, want {text:hi}", u.Content)
	}
	if u.ContentItems != nil {
		t.Fatalf("ContentItems = %#v, want nil", u.ContentItems)
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

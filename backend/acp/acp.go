package acp

import (
	"encoding/json"
	"fmt"
)

// Wire model of the Agent Client Protocol, per
// https://agentclientprotocol.com (v1 schema). ACP-defined keys are camelCase;
// discriminator string values are snake_case. Extras stay tolerant via any.

// ProtocolVersion is the negotiated protocol version (baseline v1).
const ProtocolVersion = 1

// Method names used by both peers.
const (
	MethodInitialize        = "initialize"
	MethodAuthenticate      = "authenticate"
	MethodSessionNew        = "session/new"
	MethodSessionLoad       = "session/load"
	MethodSessionPrompt     = "session/prompt"
	MethodSessionCancel     = "session/cancel"
	MethodSessionUpdate     = "session/update"
	MethodSessionSetMode    = "session/set_mode"
	MethodRequestPermission = "session/request_permission"
	MethodFsReadTextFile    = "fs/read_text_file"
	MethodFsWriteTextFile   = "fs/write_text_file"
)

// Content block types (ContentBlock discriminator values).
const (
	BlockTypeText         = "text"
	BlockTypeImage        = "image"
	BlockTypeAudio        = "audio"
	BlockTypeResourceLink = "resource_link"
	BlockTypeResource     = "resource"
)

// sessionUpdate discriminator values.
const (
	UpdateAgentMessageChunk = "agent_message_chunk"
	UpdateUserMessageChunk  = "user_message_chunk"
	UpdateAgentThoughtChunk = "agent_thought_chunk"
	UpdateToolCall          = "tool_call"
	UpdateToolCallUpdate    = "tool_call_update"
	UpdatePlan              = "plan"
	UpdateAvailableCommands = "available_commands_update"
	UpdateCurrentModeUpdate = "current_mode_update"
)

// Tool kinds and statuses.
const (
	ToolKindRead    = "read"
	ToolKindEdit    = "edit"
	ToolKindDelete  = "delete"
	ToolKindMove    = "move"
	ToolKindSearch  = "search"
	ToolKindExecute = "execute"
	ToolKindThink   = "think"
	ToolKindFetch   = "fetch"
	ToolKindOther   = "other"

	ToolStatusPending    = "pending"
	ToolStatusInProgress = "in_progress"
	ToolStatusCompleted  = "completed"
	ToolStatusFailed     = "failed"
)

// StopReason values on the session/prompt response.
const (
	StopReasonEndTurn         = "end_turn"
	StopReasonMaxTokens       = "max_tokens"
	StopReasonMaxTurnRequests = "max_turn_requests"
	StopReasonRefusal         = "refusal"
	StopReasonCancelled       = "cancelled"
)

// FsCapability mirrors fs capabilities the client advertises (camelCase per v1
// schema; request_permission is inherent to session/request_permission and not
// an fs capability field).
type FsCapability struct {
	ReadTextFile  bool `json:"readTextFile,omitempty"`
	WriteTextFile bool `json:"writeTextFile,omitempty"`
}

// ClientCapabilities is the client half of initialize negotiation.
type ClientCapabilities struct {
	FileSystem FsCapability `json:"fs"`
}

// AgentCapabilities is the agent half of initialize negotiation.
type AgentCapabilities struct {
	LoadSession bool `json:"loadSession,omitempty"`
}

// PromptItem is a text content block for session/prompt prompts.
type PromptItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// NewPromptParams builds session/prompt params: {sessionId, prompt: [...] }.
func NewPromptParams(sessionID string, prompt []PromptItem) map[string]any {
	return map[string]any{"sessionId": sessionID, "prompt": prompt}
}

// PromptResponse is the session/prompt result.
type PromptResponse struct {
	StopReason string `json:"stopReason"`
}

// ContentBlock is the ACP content union; baseline Text handling here.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ToolCall models the tool-call payload. Nested form (v1 schema
// session/request_permission params.toolCall, a ToolCallUpdate).
type ToolCall struct {
	ID        string         `json:"id"`
	Title     string         `json:"title,omitempty"`
	Kind      string         `json:"kind,omitempty"`
	Status    string         `json:"status,omitempty"`
	Content   []ContentBlock `json:"content,omitempty"`
	Locations any            `json:"locations,omitempty"` // []ToolCallLocation, flexible
	RawInput  any            `json:"rawInput,omitempty"`
	RawOutput any            `json:"rawOutput,omitempty"`
}

// SessionUpdate is a session/update discriminated variant. Per the v1 schema
// the tool_call / tool_call_update variants are flat on the wire (toolCallId,
// title, kind, status...); ToolCall additionally exposes the nested form used
// by session/request_permission.
//
// Chunk-merge invariant: the frontend (frontend/src/state/agent.tsx onMsg)
// appends subsequent agent_message_chunk payloads to the first streamed
// message, so every chunk update must be decoded through the SAME fields the
// first chunk used (Content for flat `content`, ContentItems for the array
// form) — a shape change mid-stream would break the tail append.
//
// Wire collision: the `content` key carries EITHER a single object (message /
// thought chunk variants) OR an array of blocks (tool_call / tool_call_update
// variants). encoding/json cannot bind two fields to one key, so (Un)marshal
// is customized: object -> Content, array -> ContentItems.
type SessionUpdate struct {
	ID            string         `json:"sessionId"`
	SessionUpdate string         `json:"sessionUpdate"`
	Content       *ContentBlock  `json:"content,omitempty"` // message/thought chunks
	ToolCallID    string         `json:"toolCallId,omitempty"`
	ToolCall      *ToolCall      `json:"toolCall,omitempty"`
	Title         string         `json:"title,omitempty"`
	Kind          string         `json:"kind,omitempty"`
	Status        string         `json:"status,omitempty"`
	ContentItems  []ContentBlock `json:"-"` // tool_call content array
	Locations     any            `json:"locations,omitempty"`
	Raw           any            `json:"rawInput,omitempty"`
	CurrentModeID string         `json:"currentModeId,omitempty"`
	Entries       any            `json:"entries,omitempty"` // plan entries
}

// MarshalJSON emits the ContentItems array under the shared `content` key when
// set (tool_call variants), deferring the rest to the standard encoder.
func (u SessionUpdate) MarshalJSON() ([]byte, error) {
	type plain SessionUpdate
	if len(u.ContentItems) == 0 {
		return json.Marshal(plain(u))
	}
	u.Content = nil
	b, err := json.Marshal(plain(u))
	if err != nil {
		return nil, err
	}
	items, err := json.Marshal(u.ContentItems)
	if err != nil {
		return nil, err
	}
	// splice "content":[...] into the object before the closing brace
	if len(b) < 2 || b[len(b)-1] != '}' {
		return nil, fmt.Errorf("acp: unexpected sessionUpdate marshal shape")
	}
	splice := append([]byte(`,"content":`), items...)
	out := make([]byte, 0, len(b)+len(splice))
	out = append(out, b[:len(b)-1]...)
	out = append(out, splice...)
	out = append(out, '}')
	return out, nil
}

// UnmarshalJSON accepts the shared `content` key as either a single object
// (chunk variants -> Content) or an array of blocks (tool_call variants ->
// ContentItems).
func (u *SessionUpdate) UnmarshalJSON(b []byte) error {
	// Extract and remove `content` first: the plain struct binds Content to
	// that key, and encoding/json would reject the array form.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return err
	}
	contentRaw, hasContent := fields["content"]
	delete(fields, "content")
	plainBytes, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	type plain SessionUpdate
	var p plain
	if err := json.Unmarshal(plainBytes, &p); err != nil {
		return err
	}
	*u = SessionUpdate(p)
	if !hasContent {
		return nil
	}
	var probe any
	if err := json.Unmarshal(contentRaw, &probe); err != nil {
		return err
	}
	switch probe.(type) {
	case []any:
		u.Content, u.ContentItems = nil, nil
		if err := json.Unmarshal(contentRaw, &u.ContentItems); err != nil {
			return err
		}
	default:
		var cb ContentBlock
		if err := json.Unmarshal(contentRaw, &cb); err != nil {
			return err
		}
		u.Content = &cb
	}
	return nil
}

// PermissionRequest models session/request_permission. Options stay flexible
// pending the exact PermissionOption union.
type PermissionRequest struct {
	Options any `json:"options,omitempty"`
}

// NewPermissionParams builds session/request_permission params:
// {sessionId, toolCall: ToolCallUpdate, options: [...]}.
func NewPermissionParams(sessionID string, options []any, toolCall ToolCall) map[string]any {
	return map[string]any{"sessionId": sessionID, "options": options, "toolCall": toolCall}
}

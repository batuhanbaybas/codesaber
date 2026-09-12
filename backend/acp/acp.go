package acp

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
type SessionUpdate struct {
	ID            string        `json:"sessionId"`
	SessionUpdate string        `json:"sessionUpdate"`
	Content       *ContentBlock `json:"content,omitempty"` // message/thought chunks
	ToolCallID    string        `json:"toolCallId,omitempty"`
	ToolCall      *ToolCall     `json:"toolCall,omitempty"`
	Title         string        `json:"title,omitempty"`
	Kind          string        `json:"kind,omitempty"`
	Status        string        `json:"status,omitempty"`
	ContentItems  []ContentBlock
	Locations     any    `json:"locations,omitempty"`
	Raw           any    `json:"rawInput,omitempty"`
	CurrentModeID string `json:"currentModeId,omitempty"`
	Entries       any    `json:"entries,omitempty"` // plan entries
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

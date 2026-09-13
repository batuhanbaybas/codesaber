// Pure timeline reducer for the agent chat: one ordered list of message /
// tool / permission items replaces the old separate messages[] and tools[]
// arrays. Kept dependency-free (no React) so vitest can exercise it directly.

export type AgentState = 'idle' | 'thinking' | 'harness-down' | 'no-harness'

export interface MessageItem {
  type: 'message'
  id: string
  role: 'user' | 'agent' | 'system'
  text: string
  kind: 'text' | 'chunk' | 'error'
}

export interface ToolItem {
  type: 'tool'
  toolCallId: string
  title: string
  kind: string
  status: string
  content: string
}

export interface PermissionItem {
  type: 'permission'
  requestId: string
  options: PermissionOption[]
  purpose?: string
  path?: string
  oldText?: string
  newText?: string
  isNew?: boolean
  truncated?: boolean
}

export type TimelineItem = MessageItem | ToolItem | PermissionItem

export interface PermissionOption {
  optionId?: string
  name?: string
  description?: string
  kind?: string
}

export interface AgentProjectState {
  status: AgentState
  timeline: TimelineItem[]
  harness: string | null
  sessionId: string | null
}

export interface TranscriptEntry {
  role: string
  text: string
  kind: string
  status?: string
  toolId?: string
  when?: string
}

let seq = 0
const nextId = () => `t${++seq}`

export const emptyAgentState = (): AgentProjectState => ({
  status: 'no-harness',
  timeline: [],
  harness: null,
  sessionId: null,
})

// reduceMsg applies an acp.msg event. Chunk invariant: the first chunk of a
// reply creates one message item; later chunks extend that tail item. A
// non-chunk message always appends.
export const reduceMsg = (
  s: AgentProjectState,
  ev: { role?: string; text: string; kind?: string },
): AgentProjectState => {
  if (!ev.text) return s
  const timeline = s.timeline.slice()
  if (ev.kind === 'chunk') {
    // Tool cards interrupt bubble merge by design: a chunk arriving when the
    // tail is a tool card starts a NEW message instead of merging with the
    // pre-tool chunk bubble.
    const tail = timeline[timeline.length - 1]
    if (tail && tail.type === 'message' && tail.role === 'agent' && tail.kind === 'chunk') {
      timeline[timeline.length - 1] = { ...tail, text: tail.text + ev.text }
      return { ...s, timeline }
    }
    timeline.push({ type: 'message', id: nextId(), role: 'agent', text: ev.text, kind: 'chunk' })
    return { ...s, timeline }
  }
  const role: MessageItem['role'] =
    ev.role === 'user' ? 'user' : ev.role === 'system' ? 'system' : 'agent'
  timeline.push({
    type: 'message',
    id: nextId(),
    role,
    text: ev.text,
    kind: ev.kind === 'error' ? 'error' : 'text',
  })
  return { ...s, timeline }
}

// reduceTool applies an acp.tool event: upsert by toolCallId in place.
export const reduceTool = (
  s: AgentProjectState,
  ev: { toolCallId: string; title?: string; kind?: string; status?: string; content?: string },
): AgentProjectState => {
  if (!ev.toolCallId) return s
  const timeline = s.timeline.slice()
  const idx = timeline.findIndex((i) => i.type === 'tool' && i.toolCallId === ev.toolCallId)
  const item: ToolItem = {
    type: 'tool',
    toolCallId: ev.toolCallId,
    title: ev.title ?? (idx >= 0 ? (timeline[idx] as ToolItem).title : ''),
    kind: ev.kind ?? (idx >= 0 ? (timeline[idx] as ToolItem).kind : 'other'),
    status: ev.status ?? (idx >= 0 ? (timeline[idx] as ToolItem).status : 'pending'),
    content: ev.content ?? (idx >= 0 ? (timeline[idx] as ToolItem).content : ''),
  }
  if (idx >= 0) timeline[idx] = item
  else timeline.push(item)
  return { ...s, timeline }
}

// reducePermission appends a permission card (dedup by requestId).
export const reducePermission = (
  s: AgentProjectState,
  p: PermissionItem,
): AgentProjectState => {
  const exists = s.timeline.some(
    (i) => i.type === 'permission' && i.requestId === p.requestId,
  )
  if (exists) return s
  return { ...s, timeline: [...s.timeline, { ...p, type: 'permission' as const }] }
}

// removePermission drops the resolved card.
export const removePermission = (s: AgentProjectState, requestId: string): AgentProjectState => ({
  ...s,
  timeline: s.timeline.filter((i) => !(i.type === 'permission' && i.requestId === requestId)),
})

// reduceStateFlip applies an acp.state change; ANY flip drops pending
// permission cards (the backend answers them server-side, so a stale card
// lingers otherwise and clicking it errors once the request is gone).
export const reduceStateFlip = (s: AgentProjectState, st: AgentState): AgentProjectState => ({
  ...s,
  status: st,
  timeline: s.timeline.filter((i) => i.type !== 'permission'),
})

// reduceTranscript rebuilds the whole timeline from persisted entries.
export const reduceTranscript = (
  s: AgentProjectState,
  entries: TranscriptEntry[],
): AgentProjectState => {
  const timeline: TimelineItem[] = []
  for (const e of entries) {
    if (e.kind === 'tool' && e.toolId) {
      timeline.push({
        type: 'tool',
        toolCallId: e.toolId,
        title: e.text,
        kind: 'other',
        status: e.status || 'completed',
        content: '',
      })
      continue
    }
    const role: MessageItem['role'] =
      e.role === 'user' ? 'user' : e.role === 'system' ? 'system' : 'agent'
    timeline.push({
      type: 'message',
      id: nextId(),
      role,
      text: e.text,
      kind: e.kind === 'error' ? 'error' : 'text',
    })
  }
  return { ...s, timeline }
}

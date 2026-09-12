import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from 'react'
import { Events } from '@wailsio/runtime'
import * as App from '../../bindings/aide/backend/app'
import type { Info as HarnessInfo } from '../../bindings/aide/backend/acp/models'
import type { Entry } from '../../bindings/aide/backend/agentstore/models'
import { useProjects } from './projects'

export type AgentState = 'idle' | 'thinking' | 'harness-down' | 'no-harness'

export interface ChatMessage {
  id: string
  role: 'user' | 'agent' | 'system'
  text: string
  kind: 'text' | 'chunk' | 'error'
}

export interface ToolCard {
  toolCallId: string
  title: string
  kind: string
  status: string
  content: string
}

export interface PendingPermission {
  requestId: string
  options: PermissionOption[]
}

export interface PermissionOption {
  optionId?: string
  name?: string
  description?: string
  kind?: string
}

interface AgentProjectState {
  status: AgentState
  messages: ChatMessage[]
  tools: ToolCard[]
  pendingPermission: PendingPermission | null
  harness: string | null
}

interface AgentContextValue {
  state: Record<string, AgentProjectState>
  harnesses: HarnessInfo[]
  send: (projectId: string, text: string) => Promise<void>
  start: (projectId: string, harnessName: string) => Promise<void>
  stop: (projectId: string) => Promise<void>
  newSession: (projectId: string) => Promise<void>
  respondPermission: (
    projectId: string,
    requestId: string,
    optionId: string,
    cancel: boolean,
  ) => Promise<void>
}

const emptyState: AgentProjectState = {
  status: 'no-harness',
  messages: [],
  tools: [],
  pendingPermission: null,
  harness: null,
}

const AgentContext = createContext<AgentContextValue | null>(null)

let idSeq = 0
const nextId = () => `m${++idSeq}`

// asPermissionOption coerces the flexible ACP permission option entries into
// the shape the panel renders.
const asPermissionOption = (o: unknown): PermissionOption | null => {
  if (typeof o !== 'object' || o === null) return null
  const m = o as Record<string, unknown>
  const optionId = typeof m.optionId === 'string' ? m.optionId : undefined
  const name = typeof m.name === 'string' ? m.name : undefined
  if (!optionId && !name) return null
  return {
    optionId,
    name,
    description: typeof m.description === 'string' ? m.description : undefined,
    kind: typeof m.kind === 'string' ? m.kind : undefined,
  }
}

// entriesToMessages maps persisted transcript entries to chat messages.
const entriesToMessages = (entries: Entry[]): ChatMessage[] =>
  entries.map((e) => ({
    id: nextId(),
    role: (e.role === 'user' || e.role === 'system'
      ? e.role
      : 'agent') as ChatMessage['role'],
    text: e.text,
    kind: e.kind === 'error' ? 'error' : 'text',
  }))

export const AgentProvider: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  const [state, setState] = useState<Record<string, AgentProjectState>>({})
  const [harnesses, setHarnesses] = useState<HarnessInfo[]>([])
  const stateRef = useRef(state)
  stateRef.current = state

  const patch = useCallback(
    (projectId: string, fn: (s: AgentProjectState) => AgentProjectState) => {
      setState((prev) => ({
        ...prev,
        [projectId]: fn(prev[projectId] ?? emptyState),
      }))
    },
    [],
  )

  // Initial harness list + transcript pull happens per active project (below);
  // here we only subscribe to events once.
  useEffect(() => {
    const onMsg = Events.On('acp.msg', (ev: any) => {
      const { projectId, role, text, kind } = (ev.data ?? {}) as {
        projectId?: string
        role?: string
        text?: string
        kind?: string
      }
      if (!projectId || !text) return
      patch(projectId, (s) => {
        // chunk: append to the tail agent message if one is streaming
        if (kind === 'chunk' && s.messages.length > 0) {
          const tail = s.messages[s.messages.length - 1]
          if (tail.role === 'agent' && tail.kind === 'chunk') {
            return {
              ...s,
              messages: [
                ...s.messages.slice(0, -1),
                { ...tail, text: tail.text + text },
              ],
            }
          }
        }
        const msgRole: ChatMessage['role'] =
          role === 'user' ? 'user' : role === 'system' ? 'system' : 'agent'
        const msgKind: ChatMessage['kind'] = kind === 'error' ? 'error' : 'text'
        return {
          ...s,
          messages: [...s.messages, { id: nextId(), role: msgRole, text, kind: msgKind }],
        }
      })
    })
    const onTool = Events.On('acp.tool', (ev: any) => {
      const { projectId, toolCallId, title, kind, status, content } = (ev.data ??
        {}) as {
        projectId?: string
        toolCallId?: string
        title?: string
        kind?: string
        status?: string
        content?: string
      }
      if (!projectId || !toolCallId) return
      patch(projectId, (s) => {
        const idx = s.tools.findIndex((t) => t.toolCallId === toolCallId)
        const card: ToolCard = {
          toolCallId,
          title: title ?? (idx >= 0 ? s.tools[idx].title : ''),
          kind: kind ?? (idx >= 0 ? s.tools[idx].kind : 'other'),
          status: status ?? (idx >= 0 ? s.tools[idx].status : 'pending'),
          content: content ?? (idx >= 0 ? s.tools[idx].content : ''),
        }
        const tools =
          idx >= 0
            ? s.tools.map((t, i) => (i === idx ? card : t))
            : [...s.tools, card]
        return { ...s, tools }
      })
    })
    const onPermission = Events.On('acp.permission', (ev: any) => {
      const { projectId, requestId, options } = (ev.data ?? {}) as {
        projectId?: string
        requestId?: string
        options?: unknown
      }
      if (!projectId || !requestId) return
      const opts = Array.isArray(options)
        ? (options.map(asPermissionOption).filter(Boolean) as PermissionOption[])
        : []
      patch(projectId, (s) => ({
        ...s,
        pendingPermission: { requestId, options: opts },
      }))
    })
    const onState = Events.On('acp.state', (ev: any) => {
      const { projectId, state: st } = (ev.data ?? {}) as {
        projectId?: string
        state?: AgentState
      }
      if (!projectId || !st) return
      patch(projectId, (s) => ({
        ...s,
        status: st,
        // permission resolved or timed out server-side; clear on state change
        pendingPermission: st === 'thinking' ? s.pendingPermission : null,
      }))
    })
    const onTranscript = Events.On('acp.transcript', (ev: any) => {
      const { projectId, entries } = (ev.data ?? {}) as {
        projectId?: string
        entries?: Entry[]
      }
      if (!projectId) return
      patch(projectId, (s) => ({
        ...s,
        messages: entriesToMessages(entries ?? []),
        tools: [],
      }))
    })
    const onRemoved = Events.On('project.removed', (ev: any) => {
      const { id } = (ev.data ?? {}) as { id?: string }
      if (!id) return
      setState((prev) => {
        const next = { ...prev }
        delete next[id]
        return next
      })
    })
    return () => {
      onMsg()
      onTool()
      onPermission()
      onState()
      onTranscript()
      onRemoved()
    }
  }, [patch])

  // Load harness list once; refresh transcript when the active project
  // changes so the panel shows persisted history on open.
  const { activeId } = useProjects()
  useEffect(() => {
    App.ACPHarnesses()
      .then((hs) => setHarnesses(hs ?? []))
      .catch(() => setHarnesses([]))
  }, [])

  useEffect(() => {
    if (!activeId) return
    App.ACPLoadTranscript(activeId)
      .then((entries) =>
        patch(activeId, (s) => ({
          ...s,
          messages: entriesToMessages(entries ?? []),
        })),
      )
      .catch(() => {})
  }, [activeId, patch])

  const send = useCallback(
    async (projectId: string, text: string) => {
      await App.ACPSendPrompt(projectId, text)
    },
    [],
  )

  const start = useCallback(
    async (projectId: string, harnessName: string) => {
      await App.ACPStart(projectId, harnessName)
      patch(projectId, (s) => ({ ...s, harness: harnessName }))
    },
    [patch],
  )

  const stop = useCallback(
    async (projectId: string) => {
      await App.ACPStop(projectId)
      patch(projectId, (s) => ({ ...s, harness: null }))
    },
    [patch],
  )

  const newSession = useCallback(
    async (projectId: string) => {
      await App.ACPNewSession(projectId)
    },
    [],
  )

  const respondPermission = useCallback(
    async (
      projectId: string,
      requestId: string,
      optionId: string,
      cancel: boolean,
    ) => {
      await App.ACPRespondPermission(projectId, requestId, optionId, cancel)
      patch(projectId, (s) => ({ ...s, pendingPermission: null }))
    },
    [patch],
  )

  return (
    <AgentContext.Provider
      value={{
        state,
        harnesses,
        send,
        start,
        stop,
        newSession,
        respondPermission,
      }}
    >
      {children}
    </AgentContext.Provider>
  )
}

export const useAgent = (): AgentContextValue => {
  const ctx = useContext(AgentContext)
  if (!ctx) throw new Error('useAgent must be used within AgentProvider')
  return ctx
}

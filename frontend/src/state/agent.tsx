import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
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
  purpose?: string
  path?: string
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
  pendingPermissions: PendingPermission[]
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
  /** One-shot prompt turn with the last agent text captured (AI commit msg). */
  generateCommit: (projectId: string, prompt: string) => Promise<string>
}

const emptyState: AgentProjectState = {
  status: 'no-harness',
  messages: [],
  tools: [],
  pendingPermissions: [],
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
        // Chunk merge invariant: the backend streams an agent reply as
        // kind:'chunk' events. The FIRST chunk must create a message with
        // kind 'chunk'; every subsequent chunk appends to that tail message
        // (agent role + kind 'chunk'). A chunk must never fall through to the
        // "create message" branch while a chunk tail exists, or the reply
        // splits into multiple bubbles.
        const tail = s.messages[s.messages.length - 1]
        if (kind === 'chunk') {
          if (tail && tail.role === 'agent' && tail.kind === 'chunk') {
            return {
              ...s,
              messages: [
                ...s.messages.slice(0, -1),
                { ...tail, text: tail.text + text },
              ],
            }
          }
          return {
            ...s,
            messages: [
              ...s.messages,
              { id: nextId(), role: 'agent', text, kind: 'chunk' },
            ],
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
      const { projectId, requestId, options, purpose, path } = (ev.data ?? {}) as {
        projectId?: string
        requestId?: string
        options?: unknown
        purpose?: string
        path?: string
      }
      if (!projectId || !requestId) return
      const opts = Array.isArray(options)
        ? (options.map(asPermissionOption).filter(Boolean) as PermissionOption[])
        : []
      patch(projectId, (s) => ({
        ...s,
        // stacked permissions: append; duplicates (same requestId) ignored
        pendingPermissions: s.pendingPermissions.some(
          (p) => p.requestId === requestId,
        )
          ? s.pendingPermissions
          : [...s.pendingPermissions, { requestId, options: opts, purpose, path }],
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
        // a state flip means the turn ended/timed out server-side; drop the
        // whole stack (the backend answers every pending request on timeout)
        pendingPermissions: st === 'thinking' ? s.pendingPermissions : [],
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

  // generateCommit runs one synchronous prompt turn and resolves with the
  // LAST agent text captured via acp.msg. The turn is considered done when
  // the backend reports an idle acp.state (or on a 30s timeout). It listens
  // with local handlers so it never races the panel's own subscriptions.
  const generateCommit = useCallback(
    async (projectId: string, prompt: string): Promise<string> => {
      let captured = ''
      let done = false
      return new Promise<string>((resolve, reject) => {
        const fail = (why: string) => {
          if (done) return
          done = true
          offMsg()
          offState()
          clearTimeout(timer)
          reject(new Error(why))
        }
        const succeed = () => {
          if (done) return
          done = true
          offMsg()
          offState()
          clearTimeout(timer)
          const text = captured.trim()
          if (text) resolve(text)
          else reject(new Error('empty reply'))
        }
        const timer = setTimeout(() => fail('timeout'), 30000)
        const offMsg = Events.On('acp.msg', (ev: any) => {
          const { projectId: pid, role, text } = (ev.data ?? {}) as {
            projectId?: string
            role?: string
            text?: string
          }
          if (pid !== projectId || !text) return
          if (role === 'agent') captured = text
        })
        const offState = Events.On('acp.state', (ev: any) => {
          const { projectId: pid, state: st } = (ev.data ?? {}) as {
            projectId?: string
            state?: AgentState
          }
          if (pid !== projectId) return
          if (st === 'idle') succeed()
          else if (st === 'harness-down' || st === 'no-harness')
            fail('harness down')
        })
        App.ACPSendPrompt(projectId, prompt).catch((e) => fail(String(e)))
      })
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
      // backend emits acp.transcript (reset marker) on respawn; also clear
      // live-streamed tools locally in case the event races
      patch(projectId, (s) => ({ ...s, tools: [] }))
    },
    [patch],
  )

  const respondPermission = useCallback(
    async (
      projectId: string,
      requestId: string,
      optionId: string,
      cancel: boolean,
    ) => {
      await App.ACPRespondPermission(projectId, requestId, optionId, cancel)
      // resolve only the matching card, not the whole stack
      patch(projectId, (s) => ({
        ...s,
        pendingPermissions: s.pendingPermissions.filter(
          (p) => p.requestId !== requestId,
        ),
      }))
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
        generateCommit,
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

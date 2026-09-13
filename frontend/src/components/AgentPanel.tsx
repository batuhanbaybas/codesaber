import React, { useEffect, useRef, useState } from 'react'
import * as App from '../../bindings/aide/backend/app'
import { useProjects } from '../state/projects'
import {
  useAgent,
  type ChatMessage,
  type PendingPermission,
  type ToolCard,
} from '../state/agent'
import MiniDiff, { diffCounts } from './MiniDiff'

const statusDot = (status: string) => {
  if (status === 'in_progress' || status === 'pending')
    return 'bg-[#e6c07b] animate-pulse'
  if (status === 'completed') return 'bg-[#7dcf9e]'
  if (status === 'failed') return 'bg-[#e5735f]'
  return 'bg-gray-500'
}

const ToolRow: React.FC<{ t: ToolCard }> = ({ t }) => (
  <div className="px-3 py-1">
    <div className="flex items-center gap-2 text-[11px] text-dim">
      <span>{'\u{1F4BF}'}</span>
      <span className={'w-1.5 h-1.5 rounded-full shrink-0 ' + statusDot(t.status)} />
      <span className="truncate" title={t.title}>
        {t.title || t.toolCallId}
      </span>
      <span className="ml-auto text-[10px] shrink-0">{t.status}</span>
    </div>
    {t.content && (
      <pre className="mt-1 ml-5 px-2 py-1 rounded bg-[#1e1f22] text-[10px] text-dim whitespace-pre-wrap break-words max-h-32 overflow-y-auto">
        {t.content}
      </pre>
    )}
  </div>
)

const Message: React.FC<{ m: ChatMessage }> = ({ m }) => {
  if (m.role === 'user') {
    return (
      <div className="flex justify-end px-3 py-1">
        <div className="max-w-[85%] rounded bg-[var(--accent)]/15 text-primary text-xs px-2.5 py-1.5 whitespace-pre-wrap break-words">
          {m.text}
        </div>
      </div>
    )
  }
  if (m.role === 'system') {
    return (
      <div className="px-3 py-1 text-center text-[10px] text-dim">
        <span className="px-2 py-0.5 rounded bg-[#1e1f22]">{m.text}</span>
      </div>
    )
  }
  return (
    <div className="px-3 py-1">
      <div
        className={
          'text-xs whitespace-pre-wrap break-words ' +
          (m.kind === 'error' ? 'text-[#e5735f]' : 'text-dim')
        }
      >
        {m.text}
      </div>
    </div>
  )
}

const AgentPanel: React.FC = () => {
  const { activeId } = useProjects()
  const { state, harnesses, send, start, stop, newSession, respondPermission } =
    useAgent()
  const [draft, setDraft] = useState('')
  const listRef = useRef<HTMLDivElement | null>(null)
  const taRef = useRef<HTMLTextAreaElement | null>(null)
  // Per-project prompt history: seeded once from the persisted transcript
  // (last 30 user entries), then appended by every composer send. idx walks
  // with ↑/↓ when the composer is empty; it equals list.length for a fresh
  // draft.
  const histRef = useRef<Record<string, string[]>>({})
  const histIdxRef = useRef<Record<string, number>>({})
  const draftRef = useRef(draft)
  draftRef.current = draft

  useEffect(() => {
    if (!activeId || histRef.current[activeId]) return
    App.ACPLoadTranscript(activeId)
      .then((entries) => {
        if (histRef.current[activeId]) return
        const list = (entries ?? [])
          .filter((e) => e.role === 'user' && e.text.trim())
          .slice(-30)
          .map((e) => e.text)
        histRef.current[activeId] = list
        histIdxRef.current[activeId] = list.length
      })
      .catch(() => {})
  }, [activeId])

  // recall steps the history cursor by dir (-1 older, +1 newer) and fills the
  // composer. It only engages when the composer is empty or already showing
  // a recalled entry, so in-progress typing is never clobbered. Returns
  // whether the cursor moved (↑/↓ carets in full textareas move it too), so
  // the keydown handler can preventDefault selectively.
  const recall = (dir: -1 | 1): boolean => {
    if (!activeId) return false
    const list = histRef.current[activeId]
    if (!list || !list.length) return false
    let idx = histIdxRef.current[activeId] ?? list.length
    if (dir === -1) {
      const cur = idx < list.length ? list[idx] : ''
      if (draftRef.current.trim() && draftRef.current !== cur) return false
      if (idx === list.length) idx = list.length - 1
      else if (idx > 0) idx -= 1
      else return false
    } else {
      if (idx >= list.length) return false
      idx += 1
    }
    histIdxRef.current[activeId] = idx
    setDraft(idx < list.length ? list[idx] : '')
    requestAnimationFrame(grow)
    return true
  }

  const st = activeId ? state[activeId] : undefined
  const running = !!st?.harness && st.status !== 'harness-down' && st.status !== 'no-harness'
  const thinking = st?.status === 'thinking'
  const pending = st?.pendingPermissions ?? []

  // Keep the chat pinned to the newest content.
  useEffect(() => {
    const el = listRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [st?.messages.length, st?.tools.length, st?.messages])

  const grow = () => {
    const ta = taRef.current
    if (!ta) return
    ta.style.height = 'auto'
    ta.style.height = Math.min(ta.scrollHeight, 140) + 'px'
  }

  const doSend = () => {
    const text = draft.trim()
    if (!text || !activeId || thinking || !running) return
    const hist = histRef.current[activeId] ?? (histRef.current[activeId] = [])
    if (hist[hist.length - 1] !== text) hist.push(text)
    histIdxRef.current[activeId] = hist.length
    setDraft('')
    requestAnimationFrame(grow)
    void send(activeId, text)
  }

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      doSend()
      return
    }
    if (e.key === 'ArrowUp') {
      if (recall(-1)) e.preventDefault()
      return
    }
    if (e.key === 'ArrowDown') {
      if (recall(1)) e.preventDefault()
    }
  }

  if (!activeId) {
    return (
      <div className="flex flex-col h-full items-center justify-center text-xs text-dim">
        No project open
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full text-xs">
      {/* Harness bar */}
      <div className="shrink-0 flex items-center gap-2 px-2 py-1.5 border-b border-panel">
        {running ? (
          <>
            <span className="px-2 py-0.5 rounded bg-[#1e1f22] text-dim">
              {'\u25CF'} {st?.harness}
            </span>
            <button
              className="no-drag px-2 py-0.5 rounded bg-[#1e1f22] text-dim hover:text-primary"
              onClick={() => void stop(activeId)}
            >
              Stop
            </button>
          </>
        ) : (
          <HarnessPicker
            harnesses={harnesses}
            onStart={(name) => void start(activeId, name)}
          />
        )}
      </div>

      {/* Status strip */}
      {(st?.status === 'harness-down' || thinking) && (
        <div
          className={
            'shrink-0 px-3 py-1 text-[11px] ' +
            (thinking
              ? 'text-dim'
              : 'bg-[#5a1d1d] text-[#ff9999] border-b border-[#ff6b6b]/30')
          }
        >
          {thinking ? 'thinking\u2026' : 'harness down — start a harness to chat'}
        </div>
      )}

      {/* Chat list */}
      <div ref={listRef} className="flex-1 min-h-0 overflow-y-auto py-1">
        {(st?.messages ?? []).map((m) => (
          <Message key={m.id} m={m} />
        ))}
        {(st?.tools ?? []).map((t) => (
          <ToolRow key={t.toolCallId} t={t} />
        ))}
      </div>

      {/* Permission cards (stacked: multiple requests can be pending at once) */}
      {pending.map((p) => (
        <div
          key={p.requestId}
          className="shrink-0 mx-2 mb-1 rounded border border-[#e6c07b]/40 bg-[#1e1f22] p-2"
        >
          {p.purpose === 'fs-write' ? (
            <DiffReviewCard
              p={p}
              onAccept={() =>
                void respondPermission(activeId, p.requestId, 'allow', false)
              }
              onReject={() =>
                void respondPermission(activeId, p.requestId, '', true)
              }
            />
          ) : (
            <>
              <div className="text-[11px] text-[#e6c07b] mb-1">
                Permission requested
              </div>
              {p.options.length === 0 ? (
                <div className="text-dim">(no options offered)</div>
              ) : (
                <div className="flex flex-col gap-1">
                  {p.options.map((o) => (
                    <button
                      key={o.optionId ?? o.name}
                      className="text-left px-2 py-1 rounded bg-[#2a2c31] hover:bg-[#373940]"
                      onClick={() =>
                        void respondPermission(
                          activeId,
                          p.requestId,
                          o.optionId ?? '',
                          false,
                        )
                      }
                    >
                      <span className="text-primary">{o.name ?? o.optionId}</span>
                      {o.description && (
                        <span className="text-dim"> — {o.description}</span>
                      )}
                    </button>
                  ))}
                  <button
                    className="text-left px-2 py-1 rounded text-[#e5735f] hover:bg-[#2a2c31]"
                    onClick={() =>
                      void respondPermission(activeId, p.requestId, '', true)
                    }
                  >
                    Reject
                  </button>
                </div>
              )}
            </>
          )}
        </div>
      ))}

      {/* Composer */}
      <div className="shrink-0 border-t border-panel p-2 flex flex-col gap-1.5">
        <textarea
          ref={taRef}
          value={draft}
          onChange={(e) => {
            setDraft(e.target.value)
            grow()
          }}
          onKeyDown={onKeyDown}
          placeholder={
            running ? 'Ask the agent\u2026' : 'Start a harness first'
          }
          disabled={!running || thinking}
          rows={1}
          className="no-drag w-full resize-none rounded bg-[#1e1f22] px-2 py-1 text-primary outline-none focus:ring-1 focus:ring-[var(--accent)] disabled:opacity-40"
        />
        <div className="flex items-center">
          <span className="text-[10px] text-dim select-none">
            {'\u2191'} history
          </span>
          <button
            className="no-drag px-2 py-0.5 rounded bg-[#1e1f22] text-dim hover:text-primary disabled:opacity-40"
            disabled={!running}
            title="Start a fresh session (transcript is kept)"
            onClick={() => void newSession(activeId)}
          >
            New Session
          </button>
          <button
            className="no-drag ml-auto px-3 py-1 rounded bg-[var(--accent)] text-[#0b0c10] font-medium disabled:opacity-40 disabled:cursor-not-allowed"
            disabled={!draft.trim() || thinking || !running}
            onClick={doSend}
          >
            Send
          </button>
        </div>
      </div>
    </div>
  )
}

// DiffReviewCard surfaces an agent fs-write request: header (create/edit +
// path + +/- counts), inline mini diff, and Reject/Accept actions mapped to
// ACPRespondPermission (cancel / allow).
const DiffReviewCard: React.FC<{
  p: PendingPermission
  onAccept: () => void
  onReject: () => void
}> = ({ p, onAccept, onReject }) => {
  const oldText = p.oldText ?? ''
  const newText = p.newText ?? ''
  const counts = diffCounts(oldText, newText)
  return (
    <>
      <div className="flex items-center gap-2 mb-1">
        <span className="text-[11px] text-[#e6c07b] font-medium">
          {p.isNew ? 'Create file' : 'Edit file'}
        </span>
        <span
          className="text-[11px] text-dim truncate min-w-0"
          title={p.path}
        >
          {p.path}
        </span>
        <span className="ml-auto shrink-0 text-[10px] font-mono">
          <span className="text-[var(--added)]">+{counts.adds}</span>{' '}
          <span className="text-[var(--danger)]">−{counts.dels}</span>
        </span>
      </div>
      <MiniDiff oldText={oldText} newText={newText} />
      {p.truncated && (
        <div className="mt-1 text-[10px] text-[#e6c07b]/80">
          preview truncated — open full diff after applying
        </div>
      )}
      <div className="flex gap-1.5 mt-1.5">
        <button
          className="no-drag px-3 py-1 rounded bg-[#2a2c31] text-[#e5735f] hover:bg-[#373940]"
          onClick={onReject}
        >
          Reject
        </button>
        <button
          className="no-drag ml-auto px-3 py-1 rounded bg-[var(--accent)] text-[#0b0c10] font-medium hover:opacity-90"
          onClick={onAccept}
        >
          Accept
        </button>
      </div>
    </>
  )
}

const HarnessPicker: React.FC<{
  harnesses: { name: string; available: boolean }[]
  onStart: (name: string) => void
}> = ({ harnesses, onStart }) => {
  const [selected, setSelected] = useState('')
  const options = harnesses.length
    ? harnesses
    : [{ name: 'opencode', available: true }]
  const current = selected || options[0]?.name || ''
  const currentInfo = options.find((h) => h.name === current)
  return (
    <>
      <select
        className="no-drag px-2 py-0.5 rounded bg-[#1e1f22] text-primary outline-none"
        value={current}
        onChange={(e) => setSelected(e.target.value)}
      >
        {options.map((h) => (
          <option key={h.name} value={h.name} disabled={!h.available}>
            {h.name}
            {h.available ? '' : ' (not installed)'}
          </option>
        ))}
      </select>
      <button
        className="no-drag px-3 py-0.5 rounded bg-[var(--accent)] text-[#0b0c10] font-medium disabled:opacity-40 disabled:cursor-not-allowed"
        disabled={!currentInfo?.available}
        onClick={() => onStart(current)}
      >
        Start
      </button>
    </>
  )
}

export default AgentPanel

import React, { useEffect, useRef, useState } from 'react'
import * as App from '../../bindings/aide/backend/app'
import type { ChangeStatus } from '../../bindings/aide/backend/git/models'
import type { LogEntry } from '../../bindings/aide/backend/git/models'
import { useProjects } from '../state/projects'
import { useAgent } from '../state/agent'
import { useGit, diffKey } from '../state/git'
import { useTabs } from '../state/tabs'

const statusLetter = (s: ChangeStatus) =>
  s === 65 /* A */ ? 'A' : s === 68 /* D */ ? 'D' : s === 85 /* U */ ? 'U' : 'M'

const letterClass = (s: ChangeStatus, untracked?: boolean) =>
  s === 65
    ? 'text-[var(--added)]'
    : s === 68
      ? 'text-[var(--danger)]'
      : untracked
        ? 'text-dim'
        : 'text-[var(--modified)]'

const relOf = (root: string, path: string) =>
  root && path.startsWith(root + '/') ? path.slice(root.length + 1) : path

// shortPath keeps the tail and elides middle directories:
// ui/src/components/…/GraphCanvas.tsx, falling back to the last two parts
// when there is nothing meaningful in-between.
const shortPath = (rel: string) => {
  const parts = rel.split('/')
  if (parts.length <= 3) return rel
  return `${parts.slice(0, 2).join('/')}/…/${parts[parts.length - 1]}`
}

// Language badge chips by extension: 2-letter chip + dim accent color.
const LANGS: Record<string, { chip: string; color: string }> = {
  go: { chip: 'GO', color: '#5db8ea' },
  ts: { chip: 'TS', color: '#5a8ddb' },
  tsx: { chip: 'TS', color: '#5a8ddb' },
  js: { chip: 'JS', color: '#d9c466' },
  jsx: { chip: 'JS', color: '#d9c466' },
  py: { chip: 'PY', color: '#7dcf9e' },
  rs: { chip: 'RS', color: '#e6a07b' },
  html: { chip: 'HT', color: '#dd7f66' },
  htm: { chip: 'HT', color: '#dd7f66' },
  cs: { chip: 'CS', color: '#9bbb5f' },
  css: { chip: 'CS', color: '#b48ecb' },
  scss: { chip: 'CS', color: '#b48ecb' },
  md: { chip: 'MD', color: '#9aa3ad' },
  json: { chip: 'JS', color: '#d9c466' },
  yaml: { chip: 'YA', color: '#8fbf7f' },
  yml: { chip: 'YA', color: '#8fbf7f' },
  sh: { chip: 'SH', color: '#89c77f' },
  toml: { chip: 'TM', color: '#b98a6e' },
  sql: { chip: 'SQ', color: '#dfb06f' },
  php: { chip: 'PH', color: '#9a93c9' },
  rb: { chip: 'RB', color: '#e07b7b' },
  java: { chip: 'JA', color: '#d98a62' },
  sol: { chip: 'SO', color: '#c9a2dd' },
}

const langBadge = (path: string) => {
  const ext = path.slice(path.lastIndexOf('.') + 1).toLowerCase()
  const l = LANGS[ext]
  if (!l) return null
  return (
    <span
      className="shrink-0 px-1 rounded-[3px] text-[9px] font-mono leading-4"
      style={{ color: l.color, backgroundColor: l.color + '22' }}
    >
      {l.chip}
    </span>
  )
}

interface Row {
  path: string
  rel: string
  letter: ChangeStatus
  staged: boolean
  untracked?: boolean
}

// timeAgo renders compact relative time for history entries.
const timeAgo = (iso: string) => {
  const t = Date.parse(iso)
  if (isNaN(t)) return iso
  const s = Math.max(1, Math.floor((Date.now() - t) / 1000))
  if (s < 60) return `${s}s ago`
  if (s < 3600) return `${Math.floor(s / 60)}m ago`
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`
  return `${Math.floor(s / 86400)}d ago`
}

const SectionHeader: React.FC<{
  label: string
  count: number
  dotClass: string
  open: boolean
  onToggle: () => void
  onQuick: () => void
  quickTitle: string
  quickGlyph: string
}> = ({ label, count, dotClass, open, onToggle, onQuick, quickTitle, quickGlyph }) => (
  <div className="group flex items-center gap-1.5 px-3 py-1 sticky top-0 bg-panel z-10">
    <button
      className="text-dim hover:text-primary flex items-center"
      onClick={onToggle}
      title={open ? 'Collapse' : 'Expand'}
    >
      <span className={'inline-block transition-transform w-3 ' + (open ? '' : '-rotate-90')}>
        ▾
      </span>
    </button>
    <span className={'w-2 h-2 rounded-full shrink-0 ' + dotClass} />
    <span className="uppercase tracking-wide text-[10px] text-dim font-semibold flex-1 select-none">
      {label} ({count})
    </span>
    <button
      className="text-dim hover:text-primary opacity-0 group-hover:opacity-100"
      title={quickTitle}
      onClick={onQuick}
    >
      {quickGlyph}
    </button>
    <span className="px-1.5 rounded-full bg-[#33363d] text-[10px] text-dim">
      {count}
    </span>
  </div>
)

const FileRow: React.FC<{
  r: Row
  stats?: { a: number; d: number }
  onRowClick: (r: Row) => void
  onOpenDiff: (r: Row) => void
}> = ({ r, stats, onRowClick, onOpenDiff }) => (
  <div
    className="flex items-center gap-2 px-3 py-[3px] text-xs cursor-pointer hover:bg-[#2a2c31]"
    title={r.staged ? 'Click to unstage' : 'Click to stage'}
    onClick={() => onRowClick(r)}
  >
    {langBadge(r.rel) ?? (
      <span className="shrink-0 px-1 text-[9px] font-mono leading-4 text-dim">··</span>
    )}
    <span
      className="text-primary truncate flex-1 hover:underline"
      title={r.rel}
      onClick={(e) => {
        e.stopPropagation()
        onOpenDiff(r)
      }}
    >
      {shortPath(r.rel)}
    </span>
    {stats && (
      <span className="font-mono text-[10px] shrink-0">
        <span className="text-[var(--added)]">+{stats.a}</span>{' '}
        <span className="text-[var(--danger)]">−{stats.d}</span>
      </span>
    )}
    <span
      className={'w-3 text-center font-mono shrink-0 ' + letterClass(r.letter, r.untracked)}
    >
      {statusLetter(r.letter)}
    </span>
  </div>
)

const seq = (rows: { path: string }[]) =>
  rows.map((r) => r.path).join('\0')

const GitPanel: React.FC = () => {
  const { activeId, projects } = useProjects()
  const { status, errors, fetchDiff, stage, unstage, commit, setError } =
    useGit()
  const { openDiffTab } = useTabs()
  const { state: agentState, generateCommit } = useAgent()
  const [branches, setBranches] = useState<string[] | null>(null)
  const [branchOpen, setBranchOpen] = useState(false)
  const [message, setMessage] = useState('')
  const [tab, setTab] = useState<'changes' | 'history'>('changes')
  const [log, setLog] = useState<LogEntry[] | null>(null)
  const [openCommit, setOpenCommit] = useState<string | null>(null)
  const [sync, setSync] = useState<{
    remote: string
    ahead: number
    behind: number
  } | null>(null)
  const [fetching, setFetching] = useState(false)
  const [pushErr, setPushErr] = useState<string | null>(null)
  const [aiBusy, setAiBusy] = useState(false)
  const [open, setOpen] = useState({
    staged: true,
    unstaged: true,
    untracked: true,
  })
  const [stats, setStats] = useState<Record<string, { a: number; d: number }>>({})
  const st = activeId ? status[activeId] : undefined
  const err = activeId ? errors[activeId] : undefined
  const project = projects.find((p) => p.id === activeId)
  const branchRef = useRef<HTMLDivElement | null>(null)
  const commitMenuRef = useRef<HTMLDivElement | null>(null)
  const [commitMenuOpen, setCommitMenuOpen] = useState(false)

  const staged = st?.staged ?? []
  const unstaged = st?.unstaged ?? []
  const untracked = st?.untracked ?? []

  const ag = activeId ? agentState[activeId] : undefined
  const agentRunning = ag?.status === 'idle' || ag?.status === 'thinking'

  const resetSide = () => {
    setBranches(null)
    setBranchOpen(false)
    setSync(null)
    setLog(null)
    setOpenCommit(null)
    setStats({})
    setPushErr(null)
    setCommitMenuOpen(false)
  }
  useEffect(resetSide, [activeId])

  useEffect(() => {
    if (!branchOpen) return
    App.GitBranches(activeId ?? '')
      .then(setBranches)
      .catch(() => setBranches([]))
  }, [activeId, branchOpen])

  // Sync pill state: refresh on activation, branch change and after fetches.
  useEffect(() => {
    if (!activeId) return
    App.GitAheadBehind(activeId)
      .then((s) => setSync(s?.remote ? { remote: s.remote, ahead: s.ahead, behind: s.behind } : null))
      .catch(() => setSync(null))
  }, [activeId, st?.branch])

  // History (GitLog) when the History tab becomes visible.
  useEffect(() => {
    if (tab !== 'history' || !activeId) return
    App.GitLog(activeId, 20)
      .then((l) => setLog(l ?? []))
      .catch(() => setLog([]))
  }, [tab, activeId, st?.branch])

  // Per-file +/- stats: lazily fetched for visible rows in capped batches.
  useEffect(() => {
    if (!activeId || tab !== 'changes') return
    const batch = (rows: Row[], staged: boolean) => {
      const missing = rows.filter((r) => !stats[r.path]).map((r) => r.path)
      for (let i = 0; i < missing.length; i += 25) {
        App.GitStats(activeId, missing.slice(i, i + 25), staged)
          .then((res) =>
            setStats((prev) => {
              const next = { ...prev }
              for (const [p, v] of Object.entries(res ?? {})) {
                if (v && v.length >= 2) next[p] = { a: v[0], d: v[1] }
              }
              return next
            }),
          )
          .catch(() => {})
      }
    }
    batch(
      staged.map((c) => ({ path: c.path, rel: '', letter: c.status, staged: true })),
      true,
    )
    batch(
      [...unstaged, ...untracked].map((c) => ({
        path: c.path,
        rel: '',
        letter: c.status,
        staged: false,
      })),
      false,
    )
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    activeId,
    tab,
    seq(staged),
    seq(unstaged),
    seq(untracked),
  ])

  // Dropdown close-on-outside-clicks for branch + commit split menus.
  useEffect(() => {
    if (!branchOpen && !commitMenuOpen) return
    const onDoc = (e: MouseEvent) => {
      if (branchOpen && !branchRef.current?.contains(e.target as Node))
        setBranchOpen(false)
      if (commitMenuOpen && !commitMenuRef.current?.contains(e.target as Node))
        setCommitMenuOpen(false)
    }
    document.addEventListener('mousedown', onDoc)
    return () => document.removeEventListener('mousedown', onDoc)
  }, [branchOpen, commitMenuOpen])

  const rel = (p: string) =>
    relOf(project?.root ? project.root : '', p)

  const toggle = (r: Row) => {
    if (!activeId) return
    if (r.staged) unstage(activeId, [r.path])
    else stage(activeId, [r.path])
  }

  const openDiff = (r: Row) => {
    if (!activeId) return
    fetchDiff(diffKey(activeId, r.path, r.staged))
    openDiffTab(activeId, r.path, r.staged)
  }

  const doFetch = () => {
    if (!activeId || fetching) return
    setFetching(true)
    setPushErr(null)
    App.GitFetch(activeId)
      .then(() => setError(activeId, ''))
      .catch((e) => setError(activeId, String(e)))
      .finally(() => {
        setFetching(false)
        App.GitAheadBehind(activeId)
          .then((s) =>
            setSync(
              s?.remote
                ? { remote: s.remote, ahead: s.ahead, behind: s.behind }
                : null,
            ),
          )
          .catch(() => {})
      })
  }

  const doCheckout = (b: string) => {
    if (!activeId || b === st?.branch) return
    setBranchOpen(false)
    setError(activeId, '')
    App.GitCheckout(activeId, b)
      .then(() => setError(activeId, ''))
      .catch((e) => setError(activeId, String(e)))
      .finally(() => {
        App.GitStatus(activeId).catch(() => {})
        setLog(null)
      })
  }

  const doCommit = async (push: boolean) => {
    if (!activeId) return
    setCommitMenuOpen(false)
    setPushErr(null)
    commit(activeId, message.trim())
      .then(() => setMessage(''))
      .then(() => (push ? App.GitPush(activeId).catch((e) => setPushErr(String(e))) : undefined))
      .catch(() => {})
  }

  const runAI = () => {
    if (!activeId || !agentRunning || aiBusy) return
    const summary = staged.map((c) => `${c.path} (${statusLetter(c.status)})`).join(', ')
    setAiBusy(true)
    generateCommit(
      activeId,
      `generate a concise conventional commit message for the currently staged changes summarized as: ${summary}`,
    )
      .then((m) => {
        const first = m.split('\n').filter(Boolean)[0] ?? ''
        setMessage(first)
      })
      .catch(() => {})
      .finally(() => setAiBusy(false))
  }

  const disabledBtn =
    'px-2 py-0.5 rounded text-dim opacity-50 cursor-not-allowed select-none'

  return (
    <div className="flex flex-col h-full text-xs">
      {err && (
        <div className="px-3 py-1 bg-[#5a1d1d] text-[#ff9999] border-b border-[#ff6b6b]/30">
          {err}
        </div>
      )}
      {/* Row 1: branch box + sync pill */}
      <div ref={branchRef} className="relative shrink-0 flex items-center gap-2 px-2 pt-2 pb-1">
        <button
          className="no-drag flex items-center gap-1.5 flex-1 min-w-0 px-2 py-1 rounded bg-[#1e1f22] border border-[#33363d] text-dim hover:text-primary overflow-hidden"
          onClick={() => setBranchOpen((o) => !o)}
          title="Switch branch"
        >
          <span className="shrink-0">✏️</span>
          <span className="truncate text-primary">{st?.branch || '(none)'}</span>
          {st?.branch && (
            <span className="truncate shrink">{`origin/${st.branch}`}</span>
          )}
          <span className="shrink-0 ml-auto">▾</span>
        </button>
        {sync && (
          <button
            className="no-drag shrink-0 px-2 py-1 rounded-full bg-[#1e1f22] border border-[#33363d] font-mono text-[10px] hover:border-[var(--accent)]"
            title="Fetch"
            onClick={doFetch}
          >
            {fetching ? '⌛' : ''}
            <span className="text-[var(--added)]">↑{sync.ahead}</span>{' '}
            <span className="text-[var(--modified)]">↓{sync.behind}</span>
          </button>
        )}
        {branchOpen && (
          <div className="absolute left-2 z-20 mt-1 min-w-[160px] max-h-48 overflow-auto rounded border border-panel bg-[#1e1f22] shadow-lg">
            {(branches ?? []).map((b) => (
              <div
                key={b}
                className={
                  'px-3 py-1 cursor-pointer hover:bg-[#2a2c31] ' +
                  (b === st?.branch ? 'text-primary' : 'text-dim')
                }
                onClick={() => doCheckout(b)}
              >
                {b}
              </div>
            ))}
            {branches !== null && branches.length === 0 && (
              <div className="px-3 py-1 text-dim">(no branches)</div>
            )}
          </div>
        )}
      </div>
      {/* Row 2: small actions */}
      <div className="shrink-0 flex items-center gap-2 px-3 pb-1">
        <button
          className="no-drag px-2 py-0.5 rounded text-dim hover:text-primary hover:bg-[#2a2c31]"
          onClick={doFetch}
          disabled={fetching}
        >
          📥 Fetch
        </button>
        <button className={disabledBtn} title="Coming soon">
          Stash
        </button>
        <button className={disabledBtn + ' ml-auto'} title="Coming soon">
          Create PR
        </button>
      </div>
      {/* Tab strip */}
      <div className="shrink-0 flex items-center gap-1 px-2 border-b border-panel">
        <button
          className={
            'px-3 py-1.5 text-xs ' +
            (tab === 'changes'
              ? 'text-primary border-b-2 border-[var(--accent)]'
              : 'text-dim hover:text-primary')
          }
          onClick={() => setTab('changes')}
        >
          Changes
          <span className="ml-1 px-1.5 rounded-full bg-[#33363d] text-[10px] text-dim">
            {staged.length + unstaged.length + untracked.length}
          </span>
        </button>
        <button
          className={
            'px-3 py-1.5 text-xs ' +
            (tab === 'history'
              ? 'text-primary border-b-2 border-[var(--accent)]'
              : 'text-dim hover:text-primary')
          }
          onClick={() => setTab('history')}
        >
          History
          {(log?.length ?? 0) > 0 && (
            <span className="ml-1 px-1.5 rounded-full bg-[#33363d] text-[10px] text-dim">
              {log?.length}
            </span>
          )}
        </button>
        <button className={`${disabledBtn} ${tab === 'history' ? '' : ''}`} title="Coming soon">
          Stashes (0)
        </button>
      </div>
      {tab === 'changes' ? (
        <div className="flex-1 min-h-0 overflow-y-auto">
          <SectionHeader
            label="Staged changes"
            count={staged.length}
            dotClass="bg-[var(--added)]"
            open={open.staged}
            onToggle={() => setOpen((o) => ({ ...o, staged: !o.staged }))}
            onQuick={() => activeId && staged.length > 0 && unstage(activeId, staged.map((c) => c.path))}
            quickTitle="Unstage all"
            quickGlyph="−"
          />
          {open.staged &&
            staged.map((c) => (
              <FileRow
                key={c.path}
                r={{ path: c.path, rel: rel(c.path), letter: c.status, staged: true }}
                stats={stats[c.path]}
                onRowClick={toggle}
                onOpenDiff={openDiff}
              />
            ))}
          {open.staged && staged.length === 0 && (
            <div className="px-3 py-1 text-dim">No staged changes</div>
          )}
          <SectionHeader
            label="Changes"
            count={unstaged.length}
            dotClass="bg-[var(--modified)]"
            open={open.unstaged}
            onToggle={() => setOpen((o) => ({ ...o, unstaged: !o.unstaged }))}
            onQuick={() => activeId && unstaged.length > 0 && stage(activeId, unstaged.map((c) => c.path))}
            quickTitle="Stage all"
            quickGlyph="+"
          />
          {open.unstaged &&
            unstaged.map((c) => (
              <FileRow
                key={c.path}
                r={{ path: c.path, rel: rel(c.path), letter: c.status, staged: false }}
                stats={stats[c.path]}
                onRowClick={toggle}
                onOpenDiff={openDiff}
              />
            ))}
          {open.unstaged && unstaged.length === 0 && (
            <div className="px-3 py-1 text-dim">No changes</div>
          )}
          <SectionHeader
            label="Untracked"
            count={untracked.length}
            dotClass="bg-[#8a8f98]"
            open={open.untracked}
            onToggle={() => setOpen((o) => ({ ...o, untracked: !o.untracked }))}
            onQuick={() => activeId && untracked.length > 0 && stage(activeId, untracked.map((c) => c.path))}
            quickTitle="Stage all"
            quickGlyph="+"
          />
          {open.untracked &&
            untracked.map((c) => (
              <FileRow
                key={c.path}
                r={{
                  path: c.path,
                  rel: rel(c.path),
                  letter: c.status,
                  staged: false,
                  untracked: true,
                }}
                stats={stats[c.path]}
                onRowClick={toggle}
                onOpenDiff={openDiff}
              />
            ))}
          {open.untracked && untracked.length === 0 && (
            <div className="px-3 py-1 text-dim">No untracked files</div>
          )}
          {/* Recent commits on branch */}
          <div className="mt-3 border-t border-panel pt-1">
            <div className="flex items-center justify-between px-3 py-1 sticky top-0 bg-panel z-10">
              <span className="uppercase tracking-wide text-[10px] text-dim font-semibold">
                Recent on {st?.branch || '?'}
              </span>
              <button
                className="text-[10px] text-dim hover:text-[var(--accent)] uppercase tracking-wide"
                onClick={() => setTab('history')}
              >
                View log
              </button>
            </div>
            {(log ? log.slice(0, 3) : []).map((e) => (
              <div key={e.hash} className="flex items-center gap-2 px-3 py-[3px] text-xs">
                <span className="w-2 h-2 rounded-full bg-[var(--added)] shrink-0" />
                <span className="text-primary truncate">{e.message.split('\n')[0]}</span>
              </div>
            ))}
          </div>
        </div>
      ) : (
        <HistoryView
          log={log}
          branch={st?.branch ?? ''}
          onOpen={() => setTab('changes')}
          openHash={openCommit}
          setOpenHash={setOpenCommit}
        />
      )}
      {/* Commit area */}
      <div className="shrink-0 border-t border-panel p-2 flex flex-col gap-1.5">
        <button
          className="no-drag px-2 py-1 rounded text-[var(--accent)] border border-[var(--accent)]/40 text-xs disabled:opacity-40 disabled:cursor-not-allowed"
          style={{ backgroundColor: 'rgba(74,91,252,0.12)' }}
          disabled={!agentRunning || aiBusy}
          title={
            agentRunning
              ? 'Generate commit message with AI (⌘I)'
              : 'start an agent session first'
          }
          onClick={runAI}
        >
          ✨ Generate Commit Message with AI{' '}
          <span className="opacity-60 font-mono">⌘I</span>
        </button>
        <textarea
          value={message}
          onChange={(e) => setMessage(e.target.value)}
          placeholder="feat: …"
          rows={3}
          className="no-drag w-full resize-none rounded bg-[#1e1f22] px-2 py-1 text-primary font-mono text-xs outline-none focus:ring-1 focus:ring-[var(--accent)] placeholder:text-dim"
        />
        <div className="flex items-center justify-between text-[10px]">
          <span className="text-dim">
            {staged.length > 0 ? `✓ ${staged.length} staged` : 'nothing staged'}
          </span>
          <span
            className={
              message.length > 50 ? 'text-[var(--modified)] font-medium' : 'text-dim'
            }
          >
            {message.length} / 72
          </span>
        </div>
        {pushErr && (
          <div className="px-2 py-1 rounded bg-[#5a1d1d] text-[#ff9999] border border-[#ff6b6b]/30">
            {pushErr}
          </div>
        )}
        <div className="flex items-center gap-1.5">
          <div ref={commitMenuRef} className="relative flex-1 flex">
            <button
              disabled={staged.length === 0 || !message.trim() || !activeId}
              onClick={() => void doCommit(false)}
              className="no-drag flex-1 px-3 py-1 rounded-l bg-[var(--accent)] text-[#0b0c10] font-medium disabled:opacity-40 disabled:cursor-not-allowed"
            >
              ✓ Commit
            </button>
            <button
              disabled={staged.length === 0 || !message.trim() || !activeId}
              onClick={() => setCommitMenuOpen((o) => !o)}
              className="no-drag px-1.5 py-1 rounded-r bg-[var(--accent)] text-[#0b0c10] border-l border-[#0b0c10]/30 disabled:opacity-40 disabled:cursor-not-allowed"
            >
              ▾
            </button>
            {commitMenuOpen && (
              <div className="absolute bottom-full mb-1 left-0 w-full min-w-[160px] rounded border border-panel bg-[#1e1f22] shadow-lg z-20">
                <button
                  className="block w-full text-left px-3 py-1.5 text-xs text-dim hover:text-primary hover:bg-[#2a2c31] disabled:opacity-40"
                  disabled={!message.trim()}
                  onClick={() => void doCommit(true)}
                >
                  Commit &amp; Push
                </button>
              </div>
            )}
          </div>
          <button
            className="no-drag px-3 py-1 rounded text-dim opacity-50 cursor-not-allowed"
            title="Coming soon"
            disabled
          >
            Stash
          </button>
        </div>
      </div>
    </div>
  )
}

const HistoryView: React.FC<{
  log: LogEntry[] | null
  branch: string
  onOpen: () => void
  openHash: string | null
  setOpenHash: (h: string | null) => void
}> = ({ log, branch, onOpen, openHash, setOpenHash }) => (
  <div className="flex-1 min-h-0 overflow-y-auto">
    <div className="flex items-center justify-between px-3 py-1.5 sticky top-0 bg-panel z-10">
      <span className="uppercase tracking-wide text-[10px] text-dim font-semibold">
        Recent on {branch}
      </span>
      <button
        className="text-[10px] text-dim hover:text-[var(--accent)] uppercase tracking-wide"
        onClick={onOpen}
      >
        View log
      </button>
    </div>
    {(log ?? []).map((e) => {
      const open = openHash === e.hash
      const first = e.message.split('\n')[0]
      return (
        <div key={e.hash} className="px-3 py-1.5 hover:bg-[#2a2c31] cursor-pointer"
          onClick={() => setOpenHash(open ? null : e.hash)}
        >
          <div className="flex items-center gap-2">
            <span className="w-2 h-2 rounded-full bg-[var(--added)] shrink-0" />
            <span className="text-primary truncate text-xs">{first}</span>
          </div>
          <div className="pl-4 text-[10px] text-dim font-mono truncate">
            {e.hash.slice(0, 8)} · {timeAgo(e.time)} · {e.author}
          </div>
          {open && (
            <div className="mt-1 ml-4 rounded bg-[#1e1f22] border border-panel px-2 py-1.5 whitespace-pre-wrap text-dim text-[11px]">
              {e.message.trim() || first}
            </div>
          )}
        </div>
      )
    })}
    {log !== null && log.length === 0 && (
      <div className="px-3 py-2 text-dim">No commits</div>
    )}
    {log === null && <div className="px-3 py-2 text-dim">Loading…</div>}
  </div>
)

export default GitPanel

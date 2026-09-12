import React, { useEffect, useRef, useState } from 'react'
import * as App from '../../bindings/aide/backend/app'
import type { ChangeStatus } from '../../bindings/aide/backend/git/models'
import { useProjects } from '../state/projects'
import { useGit, diffKey } from '../state/git'
import { useTabs } from '../state/tabs'

const statusLetter = (s: ChangeStatus) =>
  s === 65 /* A */ ? 'A' : s === 68 /* D */ ? 'D' : s === 85 /* U */ ? 'U' : 'M'

const letterColor = (s: ChangeStatus) =>
  s === 65
    ? 'text-[#7dcf9e]'
    : s === 68
      ? 'text-[#e5735f]'
      : s === 85
        ? 'text-dim'
        : 'text-[#e6c07b]'

const relOf = (root: string, path: string) =>
  root && path.startsWith(root + '/')
    ? path.slice(root.length + 1)
    : path

interface Row {
  path: string
  rel: string
  letter: ChangeStatus
  staged: boolean
}

const Section: React.FC<{
  label: string
  badge: string
  badgeClass: string
  rows: Row[]
  onRowClick: (r: Row) => void
  onOpenDiff: (r: Row) => void
}> = ({ label, badge, badgeClass, rows, onRowClick, onOpenDiff }) => (
  <div className="mb-2">
    <div className="flex items-center gap-2 px-3 py-1 sticky top-0 bg-panel">
      <span className="text-dim font-medium">{label}</span>
      <span className={'px-1.5 rounded text-[10px] ' + badgeClass}>{badge}</span>
    </div>
    {rows.map((r) => (
      <div
        key={r.path}
        className="flex items-center gap-2 px-3 py-0.5 text-xs cursor-pointer hover:bg-[#2a2c31]"
        title={r.staged ? 'Click to unstage' : 'Click to stage'}
        onClick={() => onRowClick(r)}
      >
        <span
          className={'w-3 text-center font-mono ' + letterColor(r.letter)}
          title={statusLetter(r.letter)}
        >
          {statusLetter(r.letter)}
        </span>
        <span
          className="text-primary truncate hover:underline"
          title={r.rel}
          onClick={(e) => {
            e.stopPropagation()
            onOpenDiff(r)
          }}
        >
          {r.rel}
        </span>
      </div>
    ))}
  </div>
)

const GitPanel: React.FC = () => {
  const { activeId } = useProjects()
  const { status, errors, fetchDiff, stage, unstage, commit } = useGit()
  const { openDiffTab } = useTabs()
  const [branches, setBranches] = useState<string[] | null>(null)
  const [branchOpen, setBranchOpen] = useState(false)
  const [message, setMessage] = useState('')
  const st = activeId ? status[activeId] : undefined
  const err = activeId ? errors[activeId] : undefined
  const branchRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    setBranches(null)
    setBranchOpen(false)
  }, [activeId])

  useEffect(() => {
    if (!branchOpen) return
    App.GitBranches(activeId ?? '')
      .then(setBranches)
      .catch(() => setBranches([]))
  }, [activeId, branchOpen])

  useEffect(() => {
    if (!branchOpen) return
    const onDoc = (e: MouseEvent) => {
      if (!branchRef.current?.contains(e.target as Node)) setBranchOpen(false)
    }
    document.addEventListener('mousedown', onDoc)
    return () => document.removeEventListener('mousedown', onDoc)
  }, [branchOpen])

  const staged = st?.staged ?? []
  const unstaged = st?.unstaged ?? []
  const untracked = st?.untracked ?? []

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

  const badge = (n: number, cls: string) => ({
    badge: String(n),
    badgeClass: n ? `bg-[#33363d] ${cls}` : 'text-dim',
  })

  return (
    <div className="flex flex-col h-full text-xs">
      {err && (
        <div className="px-3 py-1 bg-[#5a1d1d] text-[#ff9999] border-b border-[#ff6b6b]/30">
          {err}
        </div>
      )}
      <div ref={branchRef} className="relative shrink-0 px-2 pt-2 pb-1">
        <button
          className="no-drag px-2 py-0.5 rounded bg-[#1e1f22] text-dim hover:text-primary"
          onClick={() => setBranchOpen((o) => !o)}
        >
          {'\u2387'} {st?.branch || '(none)'}
        </button>
        {branchOpen && (
          <div className="absolute left-2 z-20 mt-1 min-w-[140px] max-h-48 overflow-auto rounded border border-panel bg-[#1e1f22] shadow-lg">
            {(branches ?? []).map((b) => (
              <div
                key={b}
                className={
                  'px-3 py-1 cursor-pointer hover:bg-[#2a2c31] ' +
                  (b === st?.branch ? 'text-primary' : 'text-dim')
                }
                onClick={() => {
                  setBranchOpen(false)
                  if (activeId && b !== st?.branch)
                    App.GitCheckout(activeId, b)
                      .catch(() => {})
                      .finally(() => fetchBranchRefresh(activeId))
                }}
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
      <div className="flex-1 min-h-0 overflow-y-auto">
        <Section
          label="Staged"
          badge={badge(staged.length, 'text-[#7dcf9e]').badge}
          badgeClass={badge(staged.length, 'text-[#7dcf9e]').badgeClass}
          rows={staged.map((c) => ({
            path: c.path,
            rel: relOf('', c.path),
            letter: c.status,
            staged: true,
          }))}
          onRowClick={toggle}
          onOpenDiff={openDiff}
        />
        <Section
          label="Changes"
          {...badge(unstaged.length, 'text-[#e6c07b]')}
          rows={unstaged.map((c) => ({
            path: c.path,
            rel: relOf('', c.path),
            letter: c.status,
            staged: false,
          }))}
          onRowClick={toggle}
          onOpenDiff={openDiff}
        />
        <Section
          label="Untracked"
          {...badge(untracked.length, 'text-dim')}
          rows={untracked.map((c) => ({
            path: c.path,
            rel: relOf('', c.path),
            letter: c.status,
            staged: false,
          }))}
          onRowClick={toggle}
          onOpenDiff={openDiff}
        />
      </div>
      <div className="shrink-0 border-t border-panel p-2 flex flex-col gap-2">
        <textarea
          value={message}
          onChange={(e) => setMessage(e.target.value)}
          placeholder="Commit message"
          rows={3}
          className="no-drag w-full resize-none rounded bg-[#1e1f22] px-2 py-1 text-primary outline-none focus:ring-1 focus:ring-[var(--accent)]"
        />
        <button
          disabled={staged.length === 0 || !message.trim() || !activeId}
          onClick={() => {
            if (!activeId) return
            commit(activeId, message.trim()).then(() => setMessage(''))
          }}
          className="no-drag self-end px-3 py-1 rounded bg-[var(--accent)] text-[#0b0c10] font-medium disabled:opacity-40 disabled:cursor-not-allowed"
        >
          Commit
        </button>
      </div>
    </div>
  )
}

// fetchBranchRefresh is a tiny helper to refresh the panel status after a
// checkout; the git.status event also arrives, this just speeds it up.
const fetchBranchRefresh = (projectId: string) => {
  App.GitStatus(projectId).catch(() => {})
}

export default GitPanel

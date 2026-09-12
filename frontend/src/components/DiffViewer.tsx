import React, { useEffect } from 'react'
import { useGit, diffKey } from '../state/git'
import { parseDiffTabPath, type Tab } from '../state/tabs'

interface Line {
  op: '+' | '-' | ' '
  text: string
  oldNo?: number
  newNo?: number
}

// parseHunks parses "@@ -a,b +c,d @@" style headers into lines carrying
// two-gutter numbers. Structure changes (no "\ No newline" handling beyond a
// skip) — parse errors render plain lines via the fallback path.
const parseHunk = (startOld: number, startNew: number, lines: string[]): Line[] => {
  const out: Line[] = []
  let o = startOld
  let n = startNew
  for (const raw of lines) {
    if (raw.startsWith('\\')) continue
    const op = raw.slice(0, 1) as '+' | '-' | ' '
    const text = raw.slice(1)
    if (op === '+') {
      out.push({ op, text, newNo: n++ })
    } else if (op === '-') {
      out.push({ op, text, oldNo: o++ })
    } else {
      out.push({ op: ' ', text, oldNo: o++, newNo: n++ })
    }
  }
  return out
}

const HUNK_RE = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)?/

const Hunk: React.FC<{ header: string; lines: string[] }> = ({
  header,
  lines,
}) => {
  let body: Line[]
  try {
    const m = HUNK_RE.exec(header)
    if (!m) throw new Error('bad header')
    body = parseHunk(parseInt(m[1], 10), parseInt(m[2], 10), lines)
  } catch {
    body = lines.map((l) => ({ op: ' ' as const, text: l }))
  }
  return (
    <>
      <div className="px-0 py-0.5 font-mono text-dim bg-[#2a2c31] select-none">
        {header}
      </div>
      {body.map((l, i) => (
        <div
          key={i}
          className={
            'flex ' +
            (l.op === '+'
              ? 'bg-[#1d3227] text-[#7dcf9e]'
              : l.op === '-'
                ? 'bg-[#3a2323] text-[#e5735f]'
                : '')
          }
        >
          <span className="w-10 shrink-0 pr-1 text-dim bg-panel text-right font-mono select-none">
            {l.oldNo ?? ''}
          </span>
          <span className="w-10 shrink-0 pr-1 text-dim bg-panel text-right font-mono select-none">
            {l.newNo ?? ''}
          </span>
          <span className="w-3 shrink-0 select-none">{l.op === ' ' ? '' : l.op}</span>
          <span className="font-mono whitespace-pre pr-2">
            {l.text === '' ? ' ' : l.text}
          </span>
        </div>
      ))}
    </>
  )
}

const DiffViewer: React.FC<{
  projectId: string
  tab: Tab
  active: boolean
}> = ({ projectId, tab, active }) => {
  const { diffs, fetchDiff } = useGit()
  const parsed = parseDiffTabPath(tab.path)
  const key = parsed ? diffKey(projectId, parsed.path, parsed.staged) : ''
  const patch = key ? diffs[key] : undefined

  // (Re)fetch whenever the tab first renders; provider caches by key.
  useEffect(() => {
    if (key && !diffs[key]) fetchDiff(key)
  }, [key, diffs, fetchDiff])

  const additions = patch?.hunks?.reduce((a, h) => a + h.additions, 0) ?? 0
  const deletions = patch?.hunks?.reduce((a, h) => a + h.deletions, 0) ?? 0

  return (
    <div
      className="absolute inset-0 flex flex-col overflow-hidden"
      style={{ display: active ? 'flex' : 'none' }}
    >
      <div className="flex items-center gap-2 px-3 py-1 text-xs bg-panel border-b border-panel shrink-0">
        <span
          className={
            parsed?.staged ? 'text-[#7dcf9e]' : 'text-[#e6c07b]'
          }
          title={parsed?.staged ? 'staged diff' : 'unstaged diff'}
        >
          {parsed?.staged ? 'staged' : 'unstaged'}
        </span>
        <span className="text-primary truncate" title={parsed?.path}>
          {parsed?.path}
        </span>
        <span className="text-[#7dcf9e]">+{additions}</span>
        <span className="text-[#e5735f]">−{deletions}</span>
      </div>
      <div className="flex-1 overflow-auto" style={{ overflowX: 'auto' }}>
        {patch ? (
          patch.hunks && patch.hunks.length > 0 ? (
            patch.hunks.map((h, i) => (
              <div
                key={i}
                className="text-[11px] leading-5 min-w-max px-0"
              >
                <Hunk header={h.header} lines={h.lines ?? []} />
              </div>
            ))
          ) : (
            <div className="px-3 py-2 text-dim">No changes to show</div>
          )
        ) : (
          <div className="px-3 py-2 text-dim">Loading diff…</div>
        )}
      </div>
    </div>
  )
}

export default DiffViewer

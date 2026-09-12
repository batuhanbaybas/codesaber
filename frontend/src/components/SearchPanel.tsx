import React, { useEffect, useRef, useState } from 'react'
import * as App from '../../bindings/aide/backend/app'
import type { Match, Result } from '../../bindings/aide/backend/search/models'
import { useProjects } from '../state/projects'
import { useTabs } from '../state/tabs'
import { langChip } from '../lib/filetype'

const relOf = (root: string, path: string) =>
  root && path.startsWith(root + '/') ? path.slice(root.length + 1) : path

// Snippet renders the match line with the matched substring highlighted
// (accent bg). Literal mode highlights the first case-folded occurrence;
// regex mode highlights the first match per line only (MVP).
const Snippet: React.FC<{ m: Match; term: string; regex: boolean }> = ({
  m,
  term,
  regex,
}) => {
  let start = -1
  let end = -1
  if (regex && term) {
    try {
      const re = new RegExp(term, 'i')
      const hit = re.exec(m.text)
      if (hit) {
        start = hit.index
        end = start + hit[0].length
      }
    } catch {
      // invalid pattern: no highlight
    }
  } else if (term) {
    const idx = m.text.toLowerCase().indexOf(term.toLowerCase())
    if (idx >= 0) {
      start = idx
      end = idx + term.length
    }
  }
  if (start < 0) return <>{m.text}</>
  return (
    <>
      {m.text.slice(0, start)}
      <span className="bg-[#3b5bfd]/45 text-white rounded-[3px]">
        {m.text.slice(start, end)}
      </span>
      {m.text.slice(end)}
    </>
  )
}

const FileCard: React.FC<{
  root: string
  fm: { path: string; matches: Match[] | null }
  term: string
  regex: boolean
  onOpen: (path: string, m: Match) => void
}> = ({ root, fm, term, regex, onOpen }) => {
  const [open, setOpen] = useState(true)
  const matches = fm.matches ?? []
  return (
    <div className="rounded-lg border border-[#333639] bg-[#242629] mb-2 overflow-hidden">
      <button
        className="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-white/4"
        onClick={() => setOpen((o) => !o)}
        title={open ? 'Collapse' : 'Expand'}
      >
        <span
          className={
            'inline-block transition-transform text-[10px] text-dim w-3 ' +
            (open ? 'rotate-90' : '')
          }
        >
          ›
        </span>
        {langChip(fm.path)}
        <span className="truncate text-primary flex-1" title={fm.path}>
          {relOf(root, fm.path)}
        </span>
        <span className="rounded-full bg-[#3a3c3f] text-[11px] px-2 h-5 flex items-center text-[#bcbec4] shrink-0">
          {matches.length}
        </span>
      </button>
      {open && (
        <div className="pb-1">
          {matches.map((m, i) => (
            <button
              key={i}
              className="w-full flex items-baseline gap-2 px-3 py-1 text-left font-mono hover:bg-white/4"
              onClick={() => onOpen(fm.path, m)}
            >
              <span className="shrink-0 text-dim tabular-nums select-none">
                {m.line}:{m.col}
              </span>
              <span className="truncate text-[#d6d7da]">
                <Snippet m={m} term={term} regex={regex} />
              </span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

const SearchPanel: React.FC = () => {
  const { activeId, projects } = useProjects()
  const { openFile } = useTabs()
  const [term, setTerm] = useState('')
  const [regex, setRegex] = useState(false)
  const [result, setResult] = useState<Result | null>(null)
  const [running, setRunning] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement | null>(null)
  // seq drops superseded responses: only the latest issued query may paint.
  const seqRef = useRef(0)
  const project = projects.find((p) => p.id === activeId)
  const root = project?.root ?? ''

  // Auto-focus on mount and whenever the rail/⌘⇧F requests it.
  useEffect(() => {
    inputRef.current?.focus()
    const onFocus = () => {
      inputRef.current?.focus()
      inputRef.current?.select()
    }
    window.addEventListener('aide:search-focus', onFocus)
    return () => window.removeEventListener('aide:search-focus', onFocus)
  }, [])

  // Debounced live search (250ms); a newer query supersedes the older one.
  useEffect(() => {
    const trimmed = term.trim()
    if (!activeId || !trimmed) {
      seqRef.current++
      setResult(null)
      setError(null)
      setRunning(false)
      return
    }
    const seq = ++seqRef.current
    setRunning(true)
    const timer = window.setTimeout(() => {
      App.SearchText(activeId, trimmed, regex)
        .then((res) => {
          if (seqRef.current !== seq) return
          setResult(res ?? null)
          setError(null)
        })
        .catch((e) => {
          if (seqRef.current !== seq) return
          setResult(null)
          setError(String(e))
        })
        .finally(() => {
          if (seqRef.current === seq) setRunning(false)
        })
    }, 250)
    return () => window.clearTimeout(timer)
  }, [term, regex, activeId])

  const onOpen = (path: string, m: Match) => {
    if (!activeId) return
    void openFile(activeId, path, {
      reveal: { line: m.line - 1, character: m.col - 1 },
    })
  }

  return (
    <div className="flex flex-col h-full text-xs bg-[#1a1b1e] px-3 py-3">
      {/* Query row: input + regex toggle + spinner */}
      <div className="shrink-0 flex items-center gap-2 mb-2">
        <div className="relative flex-1 min-w-0">
          <input
            ref={inputRef}
            className="no-drag w-full h-9 px-3 pr-8 rounded-lg border border-[#333639] bg-[#242629] text-primary placeholder:text-dim outline-none focus:border-[#4a4f55]"
            placeholder="Search project…"
            value={term}
            onChange={(e) => setTerm(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Escape') setTerm('')
            }}
            aria-label="Search project"
          />
          {running && (
            <span
              className="absolute right-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 rounded-full border-2 border-[#4a4f55] border-t-[var(--accent)] animate-spin"
              aria-label="Searching"
            />
          )}
        </div>
        <button
          className={
            'no-drag h-9 px-2.5 rounded-lg border font-mono shrink-0 ' +
            (regex
              ? 'border-[var(--accent)] text-primary bg-white/10'
              : 'border-[#333639] bg-[#242629] text-dim hover:text-primary hover:border-[#4a4f55]')
          }
          title="Regular expression"
          aria-label="Toggle regular expression mode"
          aria-pressed={regex}
          onClick={() => setRegex((r) => !r)}
        >
          .*
        </button>
      </div>

      {/* Count line */}
      <div className="shrink-0 flex items-center text-dim mb-2 min-h-4">
        {error ? (
          <span className="text-[#ff9999]">{error}</span>
        ) : term.trim() ? (
          <>
            {running && !result
              ? 'Searching…'
              : result
                ? `${result.matches} results in ${result.files} files` +
                  (result.truncated ? ' (capped)' : '')
                : ''}
          </>
        ) : (
          'Type to search across project files'
        )}
      </div>

      {/* Results */}
      <div className="flex-1 min-h-0 overflow-auto">
        {(result?.filesMatches ?? []).map((fm) => (
          <FileCard
            key={fm.path}
            root={root}
            fm={fm}
            term={term.trim()}
            regex={regex}
            onOpen={onOpen}
          />
        ))}
      </div>
    </div>
  )
}

export default SearchPanel

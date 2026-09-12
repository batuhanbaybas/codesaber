import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import { useProjects } from '../state/projects'
import { useTabs } from '../state/tabs'
import * as App from '../../bindings/aide/backend/app'

interface FileItem {
  projectId: string
  projectName: string
  root: string
  path: string
  name: string
  rel: string
}

const maxResults = 50

// fuzzyScore returns a match score for query against path (case-insensitive
// subsequence with a contiguous-run bonus), or -1 when not a subsequence.
// Shorter paths win ties.
const fuzzyScore = (query: string, path: string): number => {
  const q = query.toLowerCase()
  const s = path.toLowerCase()
  let qi = 0
  let score = 0
  let prev = -2
  for (let i = 0; i < s.length && qi < q.length; i++) {
    if (s[i] === q[qi]) {
      score += 1 + (i === prev + 1 ? 2 : 0)
      prev = i
      qi++
    }
  }
  if (qi < q.length) return -1
  return score
}

const QuickOpen: React.FC = () => {
  const { projects, setActive } = useProjects()
  const { openFile, tabsByProject } = useTabs()
  const [openState, setOpenState] = useState(false)
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState(0)
  const [files, setFiles] = useState<FileItem[]>([])
  const inputRef = useRef<HTMLInputElement>(null)
  const cacheRef = useRef<Map<string, FileItem[]>>(new Map())

  const close = useCallback(() => {
    setOpenState(false)
    setQuery('')
    setSelected(0)
  }, [])

  // Mod-P opens/toggles the quick open. The command palette takes priority:
  // when it is on screen Mod-P is ignored so the two overlays never stack.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && !e.shiftKey && e.key.toLowerCase() === 'p') {
        e.preventDefault()
        if (document.querySelector('[data-command-palette]')) return
        setOpenState((prev) => {
          if (prev) {
            setQuery('')
            setSelected(0)
            return false
          }
          return true
        })
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  // 'aide:quickopen' (activity rail) opens the palette too. Same priority
  // rule as Mod-P: never stack on top of the command palette.
  useEffect(() => {
    const onOpen = () => {
      if (document.querySelector('[data-command-palette]')) return
      setOpenState(true)
    }
    window.addEventListener('aide:quickopen', onOpen)
    return () => window.removeEventListener('aide:quickopen', onOpen)
  }, [])

  useEffect(() => {
    if (openState) inputRef.current?.focus()
  }, [openState])

  // Lazily index every open project via the backend walker; results are
  // cached per project root until the project set changes.
  useEffect(() => {
    if (!openState) return
    const roots = new Set(projects.map((p) => p.root))
    for (const root of cacheRef.current.keys()) {
      if (!roots.has(root)) cacheRef.current.delete(root)
    }
    let disposed = false
    const collected: FileItem[] = []
    const apply = () => {
      if (!disposed) setFiles([...collected])
    }
    for (const p of projects) {
      const cached = cacheRef.current.get(p.root)
      if (cached) {
        collected.push(...cached)
        apply()
        continue
      }
      void App.IndexFiles(p.root)
        .then((paths) => {
          if (disposed) return
          const items: FileItem[] = (paths ?? []).map((path) => ({
            projectId: p.id,
            projectName: p.name,
            root: p.root,
            path,
            name: path.slice(path.lastIndexOf('/') + 1),
            rel: path.startsWith(p.root + '/')
              ? path.slice(p.root.length + 1)
              : path,
          }))
          cacheRef.current.set(p.root, items)
          collected.push(...items)
          apply()
        })
        .catch(() => {})
    }
    return () => {
      disposed = true
    }
  }, [openState, projects])

  // Empty query shows last-opened files: most recently active tab first,
  // projects in recency order.
  const recent = useMemo<FileItem[]>(() => {
    const out: FileItem[] = []
    for (const p of projects) {
      const st = tabsByProject[p.id]
      if (!st) continue
      const ordered = [
        ...st.open.filter((t) => t.path === st.active),
        ...st.open.filter((t) => t.path !== st.active),
      ]
      for (const t of ordered) {
        out.push({
          projectId: p.id,
          projectName: p.name,
          root: p.root,
          path: t.path,
          name: t.title,
          rel: t.path.startsWith(p.root + '/')
            ? t.path.slice(p.root.length + 1)
            : t.path,
        })
      }
    }
    return out
  }, [projects, tabsByProject])

  const results = useMemo<FileItem[]>(() => {
    if (query === '') return recent
    const scored: { item: FileItem; score: number }[] = []
    for (const item of files) {
      const score = fuzzyScore(query, item.rel)
      if (score >= 0) scored.push({ item, score })
    }
    scored.sort(
      (a, b) =>
        b.score - a.score ||
        a.item.rel.length - b.item.rel.length ||
        (a.item.rel < b.item.rel ? -1 : 1),
    )
    return scored.slice(0, maxResults).map((s) => s.item)
  }, [query, files, recent])

  const selectIdx = Math.min(selected, Math.max(0, results.length - 1))

  const pick = useCallback(
    (item: FileItem) => {
      setActive(item.projectId)
      void openFile(item.projectId, item.path)
      close()
    },
    [setActive, openFile, close],
  )

  const onInputKey = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.preventDefault()
      close()
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      setSelected((i) => Math.min(i + 1, results.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setSelected((i) => Math.max(i - 1, 0))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      const item = results[selectIdx]
      if (item) pick(item)
    }
  }

  if (!openState) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-black/40"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) close()
      }}
    >
      <div
        className="mt-[12vh] w-[560px] max-w-[80vw] rounded-lg bg-panel border border-panel shadow-2xl overflow-hidden"
        onKeyDown={onInputKey}
      >
        <input
          ref={inputRef}
          value={query}
          onChange={(e) => {
            setQuery(e.target.value)
            setSelected(0)
          }}
          placeholder="Search files by name…"
          className="w-full px-3 py-2.5 bg-transparent outline-none text-primary text-[13px] border-b border-panel placeholder:text-dim"
        />
        <div className="max-h-[40vh] overflow-y-auto py-1">
          {results.map((item, i) => (
            <div
              key={item.projectId + '\0' + item.path}
              className={
                'px-3 py-1.5 cursor-default ' +
                (i === selectIdx ? 'bg-[#373940]' : 'hover:bg-[#2e3037]')
              }
              onMouseEnter={() => setSelected(i)}
              onClick={() => pick(item)}
            >
              <div className="text-[12px] text-primary truncate">{item.name}</div>
              <div className="text-[10px] text-dim truncate">
                {item.projectName}/{item.rel}
              </div>
            </div>
          ))}
          {results.length === 0 && (
            <div className="px-3 py-2 text-dim text-[11px]">No matching files</div>
          )}
        </div>
        <div className="px-3 py-1.5 border-t border-panel text-dim text-[10px]">
          {'\u21b5 to open \u00b7 esc to close'}
        </div>
      </div>
    </div>
  )
}

export default QuickOpen

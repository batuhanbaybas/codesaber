import React, { useEffect, useMemo, useRef, useState } from 'react'
import { Events } from '@wailsio/runtime'
import * as App from '../../bindings/codesaber/backend/app'
import type { Entry } from '../../bindings/codesaber/backend/models'
import { useProjects } from '../state/projects'
import { useTabs } from '../state/tabs'
import FileIcon from './FileIcon'

const MAX_CHILDREN = 200

interface Row {
  key: string
  name: string
  path: string
  dir: boolean
  depth: number
  more?: boolean
}

interface Creating {
  parent: string
  depth: number
}

const parentOf = (p: string) => {
  const i = p.lastIndexOf('/')
  return i <= 0 ? '/' : p.slice(0, i)
}

const FileTree: React.FC<{ root: string; projectId: string }> = ({
  root,
  projectId,
}) => {
  const { setActive } = useProjects()
  const { openFile } = useTabs()
  const [entries, setEntries] = useState<Entry[]>([])
  const [openDirs, setOpenDirs] = useState<Set<string>>(new Set())
  const [extra, setExtra] = useState<Record<string, Entry[]>>({})
  const [creating, setCreating] = useState<Creating | null>(null)
  const [draft, setDraft] = useState('')
  const inputRef = useRef<HTMLInputElement | null>(null)
  const mountedRef = useRef(true)
  const openDirsRef = useRef(openDirs)
  openDirsRef.current = openDirs
  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  useEffect(() => {
    if (!root) return
    let disposed = false
    setEntries([])
    setOpenDirs(new Set())
    setExtra({})
    App.ListTree(root).then((es) => {
      if (!disposed) setEntries(es ?? [])
    })
    return () => {
      disposed = true
    }
  }, [root])

  const depthOf = (p: string) =>
    p === root ? 0 : p.slice(root.length).split('/').filter(Boolean).length

  // refresh re-fetches the root listing plus any expanded deep dirs so
  // watcher-driven fs.change events keep the tree current.
  const refresh = () => {
    if (!mountedRef.current) return
    App.ListTree(root).then((es) => {
      if (mountedRef.current) setEntries(es ?? [])
    })
    for (const d of openDirsRef.current) {
      if (depthOf(d) < 2) continue
      App.ListTree(d).then((es) => {
        if (mountedRef.current)
          setExtra((prev) => ({ ...prev, [d]: es ?? [] }))
      })
    }
  }

  // fs.change → debounced refresh (watchers can burst during saves).
  useEffect(() => {
    if (!root) return
    let timer: number | undefined
    const off = Events.On('fs.change', (ev: any) => {
      const p = (ev.data ?? {}) as { path?: string }
      if (typeof p.path === 'string' && !p.path.startsWith(root)) return
      window.clearTimeout(timer)
      timer = window.setTimeout(refresh, 250)
    })
    return () => {
      off()
      window.clearTimeout(timer)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [root])

  // Sidebar "+" button → start creation at the project root.
  useEffect(() => {
    const onNew = (e: Event) => {
      if ((e as CustomEvent).detail?.projectId !== projectId) return
      startCreate(root)
    }
    window.addEventListener('codesaber:filetree-new', onNew)
    return () => window.removeEventListener('codesaber:filetree-new', onNew)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId, root])

  const visibleRows = useMemo(() => {
    const rows: Row[] = []
    const emitted = new Set<string>()
    const closed: string[] = []
    const childCount: Record<string, number> = {}
    const push = (e: Entry) => {
      const parent = parentOf(e.path)
      const n = childCount[parent] ?? 0
      if (n >= MAX_CHILDREN) {
        if (n === MAX_CHILDREN) {
          childCount[parent] = n + 1
          rows.push({
            key: parent + '::more',
            name: '\u2026',
            path: parent + '::more',
            dir: false,
            depth: depthOf(e.path),
            more: true,
          })
        }
        return
      }
      childCount[parent] = n + 1
      rows.push({
        key: e.path,
        name: e.name,
        path: e.path,
        dir: e.dir,
        depth: depthOf(e.path),
      })
    }
    const visit = (e: Entry) => {
      if (emitted.has(e.path)) return
      if (closed.some((p) => e.path.startsWith(p + '/'))) return
      emitted.add(e.path)
      push(e)
      if (e.dir && !openDirs.has(e.path)) {
        closed.push(e.path)
      } else if (e.dir) {
        for (const c of extra[e.path] ?? []) visit(c)
      }
    }
    for (const e of entries) {
      if (e.path === root) continue
      visit(e)
    }
    return rows
  }, [entries, openDirs, extra, root])

  const toggle = (path: string) => {
    const wasOpen = openDirs.has(path)
    setOpenDirs((prev) => {
      const next = new Set(prev)
      if (wasOpen) next.delete(path)
      else next.add(path)
      return next
    })
    if (!wasOpen && depthOf(path) >= 2 && !extra[path]) {
      App.ListTree(path).then((es) => {
        if (mountedRef.current) setExtra((prev) => ({ ...prev, [path]: es ?? [] }))
      })
    }
  }

  // startCreate opens the inline name input under parent's children. A dir
  // parent is expanded first so the input appears in a visible spot.
  const startCreate = (parent: string) => {
    if (parent !== root && !openDirsRef.current.has(parent)) {
      setOpenDirs((prev) => new Set(prev).add(parent))
      App.ListTree(parent).then((es) => {
        if (mountedRef.current)
          setExtra((prev) => ({ ...prev, [parent]: es ?? [] }))
      })
    }
    setDraft('')
    setCreating({ parent, depth: depthOf(parent) + 1 })
  }

  const cancelCreate = () => {
    setCreating(null)
    setDraft('')
  }

  // submitCreate: trailing "/" → folder, otherwise file (nested segments in
  // the name create intermediate dirs on the backend). A created file is
  // opened in a tab right away.
  const submitCreate = () => {
    if (!creating) return
    const raw = draft.trim()
    cancelCreate()
    if (!raw) return
    const isFolder = raw.endsWith('/')
    const clean = isFolder ? raw.replace(/\/+$/, '') : raw
    if (!clean) return
    const path = creating.parent + '/' + clean
    const run = isFolder ? App.CreateFolder(path) : App.CreateFile(path)
    void run
      .then(() => {
        if (!isFolder) {
          setActive(projectId)
          void openFile(projectId, path)
        }
      })
      .catch((e) => {
        window.dispatchEvent(
          new CustomEvent('codesaber:status-hint', {
            detail: String(e).slice(0, 120),
          }),
        )
      })
  }

  useEffect(() => {
    if (creating) inputRef.current?.focus()
  }, [creating])

  const onRowClick = (row: Row) => {
    if (row.more) return
    if (row.dir) toggle(row.path)
    else {
      setActive(projectId)
      void openFile(projectId, row.path)
    }
  }

  // insertIndexFor returns the visibleRows index after the last row inside
  // parent's subtree — where the inline create input should appear.
  const insertIndexFor = (parent: string) => {
    const prefix = parent + '/'
    let idx = -1
    visibleRows.forEach((r, i) => {
      if (r.path === parent || r.path.startsWith(prefix)) idx = i
    })
    return idx + 1
  }

  const rowEls = visibleRows.map((row) => (
    <div
      key={row.key}
      className="flex items-center gap-1 py-[3px] pr-2 hover:bg-[#373940] cursor-default"
      style={{ paddingLeft: 8 + row.depth * 12 }}
      onClick={() => onRowClick(row)}
      onContextMenu={(e) => {
        e.preventDefault()
        e.stopPropagation()
        startCreate(row.dir ? row.path : parentOf(row.path))
      }}
    >
      <span className="w-2 text-dim text-[9px]">
        {row.dir ? (openDirs.has(row.path) ? '\u25be' : '\u25b8') : ''}
      </span>
      <FileIcon path={row.path} dir={row.dir} open={openDirs.has(row.path)} />
      <span className={row.more || !row.dir ? 'text-dim' : 'text-primary'}>
        {row.name}
      </span>
    </div>
  ))

  if (creating) {
    rowEls.splice(insertIndexFor(creating.parent), 0, (
      <div
        key="::create"
        className="flex items-center py-[2px] pr-2"
        style={{ paddingLeft: 8 + creating.depth * 12 }}
        onClick={(e) => e.stopPropagation()}
      >
        <input
          ref={inputRef}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') submitCreate()
            else if (e.key === 'Escape') cancelCreate()
          }}
          onBlur={cancelCreate}
          placeholder="name — trailing / for folder"
          spellCheck={false}
          className="w-full bg-[#1e1f22] border border-[var(--accent)] rounded px-1.5 py-[2px] text-[11px] text-primary outline-none"
        />
      </div>
    ))
  }

  return (
    <div
      className="py-1"
      onContextMenu={(e) => {
        e.preventDefault()
        startCreate(root)
      }}
    >
      {rowEls}
      {visibleRows.length === 0 && !creating && (
        <div className="px-3 py-2 text-dim text-[10px]">Empty</div>
      )}
    </div>
  )
}

export default FileTree

import React, { useEffect, useMemo, useRef, useState } from 'react'
import * as App from '../../bindings/aide/backend/app'
import type { Entry } from '../../bindings/aide/backend/models'
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

const FileTree: React.FC<{ root: string; projectId: string }> = ({
  root,
  projectId,
}) => {
  const { setActive } = useProjects()
  const { openFile } = useTabs()
  const [entries, setEntries] = useState<Entry[]>([])
  const [openDirs, setOpenDirs] = useState<Set<string>>(new Set())
  const [extra, setExtra] = useState<Record<string, Entry[]>>({})
  const mountedRef = useRef(true)
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

  const visibleRows = useMemo(() => {
    const rows: Row[] = []
    const emitted = new Set<string>()
    const closed: string[] = []
    const childCount: Record<string, number> = {}
    const parentOf = (p: string) => {
      const i = p.lastIndexOf('/')
      return i <= 0 ? '/' : p.slice(0, i)
    }
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

  const onRowClick = (row: Row) => {
    if (row.more) return
    if (row.dir) toggle(row.path)
    else {
      setActive(projectId)
      void openFile(projectId, row.path)
    }
  }

  return (
    <div className="py-1">
      {visibleRows.map((row) => (
        <div
          key={row.key}
          className="flex items-center gap-1 py-[3px] pr-2 hover:bg-[#373940] cursor-default"
          style={{ paddingLeft: 8 + row.depth * 12 }}
          onClick={() => onRowClick(row)}
        >
          <span className="w-2 text-dim text-[9px]">
            {row.dir ? (openDirs.has(row.path) ? '\u25be' : '\u25b8') : ''}
          </span>
          <FileIcon path={row.path} dir={row.dir} open={openDirs.has(row.path)} />
          <span className={row.more || !row.dir ? 'text-dim' : 'text-primary'}>
            {row.name}
          </span>
        </div>
      ))}
      {visibleRows.length === 0 && (
        <div className="px-3 py-2 text-dim text-[10px]">Empty</div>
      )}
    </div>
  )
}

export default FileTree

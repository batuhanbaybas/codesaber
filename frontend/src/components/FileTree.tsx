import React, { useEffect, useMemo, useState } from 'react'
import * as App from '../../bindings/aide/backend/app'
import type { Entry } from '../../bindings/aide/backend/models'

const FileTree: React.FC<{ root: string }> = ({ root }) => {
  const [entries, setEntries] = useState<Entry[]>([])
  const [openDirs, setOpenDirs] = useState<Set<string>>(new Set())

  useEffect(() => {
    if (!root) return
    let disposed = false
    App.ListTree(root).then((es) => {
      if (!disposed) setEntries(es ?? [])
    })
    return () => {
      disposed = true
    }
  }, [root])

  const visibleEntries = useMemo(() => {
    const out: Entry[] = []
    const hide = (e: Entry) =>
      e.dir && !openDirs.has(e.path) && e.path !== root
    // entries arrive dirs-first, depth-ordered; a closed dir hides its children
    const closedPrefixes: string[] = []
    for (const e of entries) {
      if (closedPrefixes.some((p) => e.path.startsWith(p + '/'))) continue
      out.push(e)
      if (hide(e)) closedPrefixes.push(e.path)
    }
    return out
  }, [entries, openDirs, root])

  const toggle = (path: string) => {
    setOpenDirs((prev) => {
      const next = new Set(prev)
      if (next.has(path)) next.delete(path)
      else next.add(path)
      return next
    })
  }

  return (
    <div className="py-1">
      {visibleEntries.map((e) => (
        <div
          key={e.path}
          className="flex items-center gap-1 py-[3px] pr-2 hover:bg-[#373940] cursor-default"
          style={{ paddingLeft: 8 + e.path.slice(root.length).split('/').length * 12 - 12 }}
          onClick={() => e.dir && toggle(e.path)}
        >
          <span className="w-2 text-dim text-[9px]">
            {e.dir ? (openDirs.has(e.path) ? '\u25be' : '\u25b8') : ''}
          </span>
          <span className={e.dir ? 'text-primary' : 'text-dim'}>{e.name}</span>
        </div>
      ))}
      {visibleEntries.length === 0 && (
        <div className="px-3 py-2 text-dim text-[10px]">Empty</div>
      )}
    </div>
  )
}

export default FileTree

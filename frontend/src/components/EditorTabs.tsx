import React, { useEffect, useState } from 'react'
import { useProjects } from '../state/projects'
import { useTabs } from '../state/tabs'
import { isMarkdown } from '../lib/mdpreview'
import FileIcon from './FileIcon'
import { MdPreviewToggle } from './MdPreview'

const EditorTabs: React.FC = () => {
  const { activeId } = useProjects()
  const { tabsByProject, setActive, close } = useTabs()
  const state = activeId ? tabsByProject[activeId] : undefined
  const tabs = state?.open ?? []
  const active = state?.active ?? null
  const activeTab = tabs.find((t) => t.path === active)
  const [ctxMenu, setCtxMenu] = useState<{
    x: number
    y: number
    path: string
  } | null>(null)

  useEffect(() => {
    if (!ctxMenu) return
    const dismiss = () => setCtxMenu(null)
    window.addEventListener('mousedown', dismiss)
    window.addEventListener('blur', dismiss)
    return () => {
      window.removeEventListener('mousedown', dismiss)
      window.removeEventListener('blur', dismiss)
    }
  }, [ctxMenu])

  return (
    <div className="flex items-stretch bg-panel text-xs h-[34px] shrink-0 overflow-x-auto">
      {tabs.map((tab) => (
        <div
          key={tab.path}
          className={
            'group relative flex items-center gap-2 px-3 border-r border-panel cursor-default shrink-0 ' +
            (tab.path === active
              ? 'bg-[#1e1f22] text-primary'
              : 'text-dim hover:text-primary hover:bg-white/4')
          }
          onClick={() => activeId && setActive(activeId, tab.path)}
          onContextMenu={(e) => {
            e.preventDefault()
            e.stopPropagation()
            if (activeId) setActive(activeId, tab.path)
            setCtxMenu({ x: e.clientX, y: e.clientY, path: tab.path })
          }}
        >
          {tab.path === active && (
            <span className="absolute top-0 left-0 right-0 h-[2px] bg-[var(--accent)]" />
          )}
          <FileIcon path={tab.path} />
          <span>{tab.title}</span>
          {tab.kind === 'diff' && (
            <span
              className={tab.diffStaged ? 'text-[#7dcf9e]' : 'text-[#e6c07b]'}
              title="diff tab"
            >
              {'\u0394'}
            </span>
          )}
          {tab.dirty && (
            <span className="text-primary group-hover:hidden">{'\u25cf'}</span>
          )}
          <button
            className="hidden group-hover:inline text-dim hover:text-primary"
            onClick={(e) => {
              e.stopPropagation()
              if (activeId) close(activeId, tab.path)
            }}
          >
            {'\u00d7'}
          </button>
        </div>
      ))}
      <button
        className="shrink-0 px-2.5 text-dim hover:text-primary"
        title="New file (coming soon)"
        aria-label="New file"
        disabled
      >
        {'\uFF0B'}
      </button>
      <div className="flex-1" />
      {activeTab && activeTab.kind !== 'diff' && isMarkdown(activeTab.path) && (
        <MdPreviewToggle path={activeTab.path} />
      )}
      {ctxMenu && (
        <div
          className="fixed z-50 min-w-[140px] rounded-md border border-[var(--bg-border)] bg-[var(--bg-panel)] shadow-lg py-1 text-[12px]"
          style={{ left: ctxMenu.x, top: ctxMenu.y }}
        >
          <button
            className="w-full text-left px-3 py-1.5 text-primary hover:bg-[#3b3d42]"
            onClick={() => {
              if (activeId) close(activeId, ctxMenu.path)
              setCtxMenu(null)
            }}
          >
            Close Tab
          </button>
        </div>
      )}
    </div>
  )
}

export default EditorTabs

import React from 'react'
import { useProjects } from '../state/projects'
import { useTabs } from '../state/tabs'
import FileIcon from './FileIcon'

const EditorTabs: React.FC = () => {
  const { activeId } = useProjects()
  const { tabsByProject, setActive, close } = useTabs()
  const state = activeId ? tabsByProject[activeId] : undefined
  const tabs = state?.open ?? []
  const active = state?.active ?? null

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
    </div>
  )
}

export default EditorTabs

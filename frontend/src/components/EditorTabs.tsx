import React from 'react'
import { useProjects } from '../state/projects'
import { useTabs } from '../state/tabs'

const EditorTabs: React.FC = () => {
  const { activeId } = useProjects()
  const { tabsByProject, setActive, close } = useTabs()
  const state = activeId ? tabsByProject[activeId] : undefined
  const tabs = state?.open ?? []
  const active = state?.active ?? null

  return (
    <div className="flex items-stretch bg-panel text-xs border-b border-panel shrink-0 min-h-[28px]">
      {tabs.map((tab) => (
        <div
          key={tab.path}
          className={
            'group flex items-center gap-2 px-3 border-r border-panel cursor-default ' +
            (tab.path === active
              ? 'bg-editor text-primary border-t-2 border-t-[var(--accent)]'
              : 'text-dim hover:text-primary')
          }
          onClick={() => activeId && setActive(activeId, tab.path)}
        >
          <span>{tab.title}</span>
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
      <div className="flex-1" />
    </div>
  )
}

export default EditorTabs

import React from 'react'
import { useProjects } from '../state/projects'
import { useTabs } from '../state/tabs'
import { useGit } from '../state/git'

const Titlebar: React.FC = () => {
  const { projects, activeId } = useProjects()
  const { tabsByProject } = useTabs()
  const { status } = useGit()

  const active = projects.find((p) => p.id === activeId) ?? null
  const branch = (activeId ? status[activeId]?.branch : undefined) ?? active?.branch
  const tabsState = activeId ? tabsByProject[activeId] : undefined
  const activeTab = tabsState?.active
    ? tabsState.open.find((t) => t.path === tabsState.active)
    : undefined

  return (
    <div
      className="drag-region h-[34px] shrink-0 flex items-center bg-panel text-xs border-b border-panel"
      style={{ paddingLeft: 72 + 'px' }}
    >
      {active ? (
        <>
          <span className="text-primary" title={active.root}>
            {active.name}
          </span>
          {branch && (
            <span
              className="no-drag ml-2 px-2 rounded bg-[#1e1f22] text-[11px] text-dim"
              title="branch"
            >
              {'\u2387'} {branch}
            </span>
          )}
          {activeTab && (
            <>
              <span className="text-dim px-2">—</span>
              <span title={activeTab.path}>{activeTab.title}</span>
            </>
          )}
        </>
      ) : (
        <span className="text-dim" title="active project">
          aide
        </span>
      )}
      <div className="flex-1" />
      <div className="no-drag flex items-center gap-1 pr-1">
        <button className="w-8 h-8 rounded text-dim hover:text-primary">+</button>
        <button className="w-8 h-8 rounded text-dim hover:text-primary">{'\u2261'}</button>
      </div>
    </div>
  )
}

export default Titlebar

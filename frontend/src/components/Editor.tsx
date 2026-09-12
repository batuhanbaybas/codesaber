import React from 'react'
import { useProjects } from '../state/projects'
import { useTabs } from '../state/tabs'

const Editor: React.FC = () => {
  const { activeId } = useProjects()
  const { tabsByProject } = useTabs()
  const state = activeId ? tabsByProject[activeId] : undefined
  const activeTab = state?.open.find((t) => t.path === state.active)

  if (!activeTab) {
    return (
      <div className="flex-1 flex items-center justify-center bg-editor min-h-0">
        <div className="w-3/4 h-1/2 border-2 border-dashed rounded-lg flex items-center justify-center text-dim">
          Editor surface {'\u2014'} placeholder for Task 10
        </div>
      </div>
    )
  }

  return (
    <div className="flex-1 min-h-0 overflow-auto bg-editor">
      <pre className="p-3 font-mono text-xs text-primary whitespace-pre">
        {activeTab.dirContent ?? ''}
      </pre>
    </div>
  )
}

export default Editor

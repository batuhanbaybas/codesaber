import React from 'react'
import FileTree from './FileTree'
import { useProjects } from '../state/projects'

const Sidebar: React.FC = () => {
  const { projects, open, remove } = useProjects()

  return (
    <div className="bg-panel w-60 shrink-0 border-r border-panel flex flex-col text-xs">
      <button
        onClick={() => void open()}
        className="no-drag mx-2 mt-2 mb-1 px-2 py-1.5 rounded bg-[#2b4d75] hover:bg-[#35597e] text-left text-[11px]"
      >
        + Add Project
      </button>
      <div className="flex-1 overflow-y-auto">
        {projects.map((p) => (
          <div key={p.id} className="mb-1 border-b border-panel pb-1">
            <div className="flex items-center gap-1 px-2 py-1.5">
              <span
                className={
                  'w-1.5 h-1.5 rounded-full shrink-0 ' +
                  (p.engineOk ? 'bg-[#4a9e6b]' : 'bg-[#c75454]')
                }
                title={p.engineOk ? 'watcher running' : 'watcher unavailable'}
              />
              <span className="flex-1 truncate text-primary" title={p.root}>
                {p.name}
              </span>
              {p.branch && (
                <span className="px-1 rounded bg-[#373940] text-dim text-[9px]">
                  {p.branch}
                </span>
              )}
              <button
                onClick={() => void remove(p.id)}
                className="no-drag px-1 text-dim hover:text-primary"
                title="Remove project"
              >
                &times;
              </button>
            </div>
            <FileTree root={p.root} />
          </div>
        ))}
        {projects.length === 0 && (
          <div className="px-3 py-2 text-dim text-[10px]">No projects open</div>
        )}
      </div>
      <div className="px-3 py-2 border-t border-panel text-dim text-[10px]">
        Phase 1 {'\u00b7'} placeholder
      </div>
    </div>
  )
}

export default Sidebar

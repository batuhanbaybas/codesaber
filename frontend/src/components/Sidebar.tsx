import React from 'react'
import FileTree from './FileTree'

const sections = [
  { name: 'Recents', entries: [], caret: '\u25b8' },
  { name: 'Open Projects', entries: [], caret: '\u25be' },
]

const Sidebar: React.FC = () => {
  return (
    <div className="bg-panel w-60 shrink-0 border-r border-panel flex flex-col text-xs">
      {sections.map((section) => (
        <div key={section.name}>
          <div className="flex items-center gap-1 px-2 py-2 text-dim uppercase tracking-wide text-[10px]">
            <span className="caret">{section.caret}</span>
            {section.name}
          </div>
        </div>
      ))}
      <div className="flex-1 overflow-y-auto">
        <FileTree />
      </div>
      <div className="px-3 py-2 border-t border-panel text-dim text-[10px]">
        Phase 1 {'\u00b7'} placeholder
      </div>
    </div>
  )
}

export default Sidebar

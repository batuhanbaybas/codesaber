import React from 'react'

interface Tab {
  name: string
  active?: boolean
  dot?: boolean
}

const demoTabs: Tab[] = [
  { name: 'main.tsx', active: true },
  { name: 'app.go' },
]

const EditorTabs: React.FC = () => {
  return (
    <div className="flex items-stretch bg-panel text-xs border-b border-panel shrink-0">
      {demoTabs.map((tab) => (
        <div
          key={tab.name}
          className={
            'flex items-center gap-2 px-3 border-r border-panel cursor-default ' +
            (tab.active ? 'bg-editor text-primary border-t-2 border-t-[var(--accent)]' : 'text-dim')
          }
        >
          <span>{tab.name}</span>
          {!tab.active && <button className="text-dim hover:text-primary">{'\u00d7'}</button>}
        </div>
      ))}
      <div className="flex-1" />
    </div>
  )
}

export default EditorTabs

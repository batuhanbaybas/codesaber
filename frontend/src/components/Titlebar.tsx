import React from 'react'

const Titlebar: React.FC = () => {
  // 72px traffic-light gutter (mac)
  return (
    <div
      className="drag-region h-11 shrink-0 flex items-center bg-panel border-b border-panel text-xs px-3"
      style={{ paddingLeft: 72 + 'px' }}
    >
      <span className="text-dim" title="active project">
        aide
      </span>
      <span className="text-dim px-2">—</span>
      <span title="open file">main.tsx</span>
      <div className="flex-1" />
      <div className="no-drag flex items-center gap-1 pr-1">
        <button className="w-8 h-8 rounded text-dim hover:text-primary">+</button>
        <button className="w-8 h-8 rounded text-dim hover:text-primary">{'\u2261'}</button>
      </div>
    </div>
  )
}

export default Titlebar

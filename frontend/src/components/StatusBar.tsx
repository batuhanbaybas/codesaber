import React from 'react'

const StatusBar: React.FC = () => {
  return (
    <div className="h-6 shrink-0 flex items-center justify-between px-3 bg-panel border-t border-panel text-[11px] text-dim">
      <div className="flex items-center gap-2">
        <span className="px-2 rounded bg-[#1e1f22] text-primary">{'\u2387'} main</span>
      </div>
      <span>aide v0.1</span>
    </div>
  )
}

export default StatusBar

import React, { useEffect, useState } from 'react'
import { Events } from '@wailsio/runtime'
import * as App from '../../bindings/aide/backend/app'
import { useLayout } from '../state/layout'

// EngineStatus mirrors the payload of the backend "engine.status" event.
interface EngineStatus {
  engine: string
  ok: boolean
  projectId?: string
}

// EnginePills tracks engine health. Baseline: a green "backend" dot after the
// first successful ListProjects round-trip, then updated by "engine.status"
// events (green ok:true, red ok:false). Unknown engines are added on arrival.
const EnginePills: React.FC = () => {
  const [statuses, setStatuses] = useState<EngineStatus[]>([])
  const [backendOk, setBackendOk] = useState(false)

  useEffect(() => {
    let disposed = false
    App.ListProjects()
      .then(() => {
        if (!disposed) setBackendOk(true)
      })
      .catch(() => {
        if (!disposed) setBackendOk(false)
      })
    return () => {
      disposed = true
    }
  }, [])

  useEffect(() => {
    const off = Events.On('engine.status', (ev: any) => {
      const st = ev.data as EngineStatus
      if (!st?.engine) return
      setStatuses((prev) => {
        const rest = prev.filter((s) => s.engine !== st.engine)
        return [...rest, st]
      })
    })
    return () => {
      off()
    }
  }, [])

  const pillEngines = ['backend', ...statuses.map((s) => s.engine)]

  return (
    <div className="flex items-center gap-2">
      {pillEngines.map((engine) => {
        const st = statuses.find((s) => s.engine === engine)
        const ok = engine === 'backend' ? backendOk : (st?.ok ?? false)
        const label =
          engine === 'backend'
            ? backendOk
              ? 'backend ok'
              : 'backend\u2026'
            : `${engine}${ok ? '' : ' down'}`
        return (
          <span
            key={engine}
            className="flex items-center gap-1 px-1.5 rounded bg-[#1e1f22]"
            title={engine === 'backend' ? 'Wails RPC' : `engine.status: ${st?.ok ? 'ok' : 'failed'}`}
          >
            <span
              className={
                'w-1.5 h-1.5 rounded-full ' +
                (ok
                  ? 'bg-[#4a9e6b]'
                  : engine === 'backend' && !st
                    ? 'bg-[#73767b]'
                    : 'bg-[#c75454]')
              }
            />
            <span className="text-dim">{label}</span>
          </span>
        )
      })}
    </div>
  )
}

const StatusBar: React.FC = () => {
  const { ui, toggle } = useLayout()
  return (
    <div className="h-6 shrink-0 flex items-center justify-between px-3 bg-panel border-t border-panel text-[11px] text-dim">
      <div className="flex items-center gap-2">
        <span className="px-2 rounded bg-[#1e1f22] text-primary">{'\u2387'} main</span>
        <EnginePills />
      </div>
      <div className="flex items-center gap-2">
        <span>aide v0.1</span>
        <button
          onClick={() => toggle('terminal')}
          className="no-drag w-5 h-4 rounded flex items-center justify-center text-dim hover:text-primary hover:bg-[#373940]"
          title={ui.terminal ? 'Hide terminal (⌘J)' : 'Show terminal (⌘J)'}
          aria-label="Toggle terminal panel"
        >
          {ui.terminal ? '\u2325' : '\u2325'}
        </button>
      </div>
    </div>
  )
}

export default StatusBar

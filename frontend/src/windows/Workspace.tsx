import React, { useEffect, useState } from 'react'
import Titlebar from '../components/Titlebar'
import Sidebar from '../components/Sidebar'
import EditorTabs from '../components/EditorTabs'
import Editor from '../components/Editor'
import CommandPalette from '../components/CommandPalette'
import QuickOpen from '../components/QuickOpen'
import StatusBar from '../components/StatusBar'
import ResizeHandle from '../components/ResizeHandle'
import AgentPanel from '../components/AgentPanel'
import { ProjectsProvider, useProjects } from '../state/projects'
import { TabsProvider } from '../state/tabs'
import { GitProvider } from '../state/git'
import { AgentProvider } from '../state/agent'
import {
  LayoutProvider,
  useLayout,
  SIZE_LIMITS,
} from '../state/layout'
import { TerminalProvider, useTerminal } from '../state/terminal'
import GitPanel from '../components/GitPanel'
import TerminalPanel from '../components/TerminalPanel'

const TerminalStrip: React.FC = () => {
  const { activeId } = useProjects()
  const { ui } = useLayout()
  const { terminalsByProject, open, close, setActive } = useTerminal()
  const state = activeId ? terminalsByProject[activeId] : undefined

  return (
    <div
      className="collapsible shrink-0 bg-panel border-t border-panel flex flex-col min-h-0"
      style={{ height: ui.terminalHeight }}
    >
      <div className="flex items-center border-b border-panel text-xs shrink-0">
        {state?.open.map((t, i) => (
          <div key={t.termId} className="flex items-center">
            <button
              className={
                'px-3 py-2 border-l-2 ' +
                (state.active === t.termId
                  ? 'border-[var(--accent)] text-primary'
                  : 'border-transparent text-dim hover:text-primary')
              }
              onClick={() => activeId && setActive(activeId, t.termId)}
            >
              {t.label ?? `sh ${i + 1}`}
            </button>
            <button
              className="mr-1 w-4 h-4 rounded flex items-center justify-center text-dim hover:text-primary hover:bg-[#373940]"
              title="Close terminal"
              aria-label={`Close ${t.label ?? `sh ${i + 1}`}`}
              onClick={() => close(t.termId)}
            >
              {'\u00D7'}
            </button>
          </div>
        ))}
        <button
          className="px-2 py-2 text-dim hover:text-primary"
          title="New terminal"
          aria-label="New terminal"
          onClick={() => open()}
        >
          {'\uFF0B'}
        </button>
      </div>
      <div className="flex-1 min-h-0 relative">
        {activeId && state?.open.length ? (
          state.open.map((t) => (
            <TerminalPanel
              key={t.termId}
              termId={t.termId}
              visible={state.active === t.termId}
            />
          ))
        ) : (
          <div className="flex h-full items-center justify-center text-dim text-xs">
            No terminal — click + to open
          </div>
        )}
      </div>
    </div>
  )
}

const WorkspaceInner: React.FC = () => {
  const { ui, toggle, setSize } = useLayout()
  const [dockTab, setDockTab] = useState<'agent' | 'git'>('git')

  // Window-level panel keybindings. Mod-P/Mod-Shift-P are owned by
  // QuickOpen/CommandPalette respectively; these only cover panels. The
  // window listener runs after CM6's keymap — preventDefault on a match is
  // enough because CodeMirror only swallows keys it binds (Mod-B/J aren't).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey) || e.shiftKey) return
      const k = e.key.toLowerCase()
      if (k === 'b') {
        e.preventDefault()
        toggle('sidebar')
      } else if (k === 'j') {
        e.preventDefault()
        toggle('terminal')
      } else if (k === 'd') {
        e.preventDefault()
        toggle('rightDock')
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [toggle])

  const railBtn = (active: boolean) =>
    'no-drag w-8 h-8 rounded flex items-center justify-center text-sm ' +
    (active ? 'text-primary bg-[#1e1f22]' : 'text-dim hover:text-primary')

  return (
    <div className="flex flex-col h-full bg-editor text-primary">
      <Titlebar />
      <div className="flex flex-1 min-h-0">
        {/* Activity rail */}
        <div className="flex flex-col items-center gap-1 py-2 bg-panel w-10 shrink-0 border-r border-panel">
          <button
            className={railBtn(ui.sidebar)}
            title="Toggle sidebar (⌘B)"
            aria-label="Toggle sidebar"
            onClick={() => toggle('sidebar')}
          >
            {'\u2B1F'}
          </button>
          <button
            className={railBtn(!ui.rightDock)}
            title="Toggle right dock (⌘D)"
            aria-label="Toggle right dock"
            onClick={() => toggle('rightDock')}
          >
            {'\u25A7'}
          </button>
          <button
            className={railBtn(ui.terminal)}
            title="Toggle terminal (⌘J)"
            aria-label="Toggle terminal"
            onClick={() => toggle('terminal')}
          >
            {'\u25BF'}
          </button>
        </div>
        {/* Left sidebar */}
        <Sidebar />
        {/* Resize handle between sidebar and editor area */}
        {ui.sidebar && (
          <ResizeHandle
            axis="x"
            onResize={(px) => setSize('sidebarWidth', px)}
            onReset={() => setSize('sidebarWidth', SIZE_LIMITS.sidebarWidth.def)}
            getCurrent={() => ui.sidebarWidth}
          />
        )}
        {/* Center editor area */}
        <div className="flex flex-col flex-1 min-w-0 bg-editor">
          <EditorTabs />
          <Editor />
        </div>
        {/* Resize handle between editor area and right dock */}
        {ui.rightDock && (
          <ResizeHandle
            axis="x"
            onResize={(px) => setSize('rightDockWidth', px)}
            onReset={() =>
              setSize('rightDockWidth', SIZE_LIMITS.rightDockWidth.def)
            }
            getCurrent={() => ui.rightDockWidth}
            flip
          />
        )}
        {/* Right dock */}
        <div
          className={
            'collapsible shrink-0 bg-panel border-l border-panel h-full flex flex-col ' +
            (ui.rightDock ? '' : 'collapsed')
          }
          style={{ width: ui.rightDock ? ui.rightDockWidth : undefined }}
        >
          <div className="flex items-center border-b border-panel text-xs">
            <button
              className={
                'px-3 py-2 border-l-2 ' +
                (dockTab === 'agent'
                  ? 'border-[var(--accent)] text-primary'
                  : 'border-transparent text-dim hover:text-primary')
              }
              onClick={() => setDockTab('agent')}
            >
              Agent
            </button>
            <button
              className={
                'px-3 py-2 border-l-2 ' +
                (dockTab === 'git'
                  ? 'border-[var(--accent)] text-primary'
                  : 'border-transparent text-dim hover:text-primary')
              }
              onClick={() => setDockTab('git')}
            >
              Git
            </button>
            <button
              onClick={() => toggle('rightDock')}
              className="no-drag ml-auto mr-1 w-5 h-5 rounded flex items-center justify-center text-dim hover:text-primary hover:bg-[#373940]"
              title="Collapse dock (⌘D)"
              aria-label="Collapse right dock"
            >
              {'\u2039'}
            </button>
          </div>
          <div className="flex-1 min-h-0 flex flex-col">
            {dockTab === 'agent' ? <AgentPanel /> : <GitPanel />}
          </div>
        </div>
      </div>
      <StatusBar />
      <CommandPalette />
      <QuickOpen />
      {/* Bottom strip: terminal */}
      {ui.terminal && (
        <>
          <ResizeHandle
            axis="y"
            flip
            onResize={(px) => setSize('terminalHeight', px)}
            onReset={() =>
              setSize('terminalHeight', SIZE_LIMITS.terminalHeight.def)
            }
            getCurrent={() => ui.terminalHeight}
          />
          <TerminalStrip />
        </>
      )}
    </div>
  )
}

const Workspace: React.FC = () => {
  return (
    <ProjectsProvider>
      <TabsProvider>
        <GitProvider>
          <AgentProvider>
            <LayoutProvider>
              <TerminalProvider>
                <WorkspaceInner />
              </TerminalProvider>
            </LayoutProvider>
          </AgentProvider>
        </GitProvider>
      </TabsProvider>
    </ProjectsProvider>
  )
}

export default Workspace

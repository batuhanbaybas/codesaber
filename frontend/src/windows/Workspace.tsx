import React, { useEffect } from 'react'
import Titlebar from '../components/Titlebar'
import Sidebar from '../components/Sidebar'
import EditorTabs from '../components/EditorTabs'
import Editor from '../components/Editor'
import CommandPalette from '../components/CommandPalette'
import StatusBar from '../components/StatusBar'
import { ProjectsProvider } from '../state/projects'
import { TabsProvider } from '../state/tabs'
import { LayoutProvider, useLayout } from '../state/layout'

const WorkspaceInner: React.FC = () => {
  const { ui, toggle } = useLayout()

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
        {/* Center editor area */}
        <div className="flex flex-col flex-1 min-w-0 bg-editor">
          <EditorTabs />
          <Editor />
        </div>
        {/* Right dock */}
        <div
          className={
            'collapsible shrink-0 bg-panel border-l border-panel h-full flex flex-col ' +
            (ui.rightDock ? 'w-64' : 'collapsed')
          }
        >
          <div className="flex items-center border-b border-panel text-xs">
            <div className="px-3 py-2 border-l-2 border-transparent text-dim">Agent</div>
            <div className="px-3 py-2 border-l-2 border-transparent text-dim">Git</div>
            <button
              onClick={() => toggle('rightDock')}
              className="no-drag ml-auto mr-1 w-5 h-5 rounded flex items-center justify-center text-dim hover:text-primary hover:bg-[#373940]"
              title="Collapse dock (⌘D)"
              aria-label="Collapse right dock"
            >
              {'\u203A'}
            </button>
          </div>
          <div className="flex-1 flex items-center justify-center text-xs text-dim">
            Empty placeholder
          </div>
        </div>
      </div>
      <StatusBar />
      <CommandPalette />
      {/* Bottom strip: terminal placeholder */}
      {ui.terminal && (
        <div className="h-40 shrink-0 bg-panel border-t border-panel px-3 py-2 text-xs text-dim">
          Terminal
        </div>
      )}
    </div>
  )
}

const Workspace: React.FC = () => {
  return (
    <ProjectsProvider>
      <TabsProvider>
        <LayoutProvider>
          <WorkspaceInner />
        </LayoutProvider>
      </TabsProvider>
    </ProjectsProvider>
  )
}

export default Workspace

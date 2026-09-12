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
import { GitProvider, useGit } from '../state/git'
import { AgentProvider } from '../state/agent'
import {
  LayoutProvider,
  useLayout,
  SIZE_LIMITS,
  type LayoutUI,
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

const railBtn = (active: boolean) =>
  'no-drag relative w-7 h-7 rounded-md flex items-center justify-center ' +
  (active
    ? 'text-primary bg-white/10 after:absolute after:left-[-6px] after:top-1 after:bottom-1 after:w-0.5 after:bg-[var(--accent)] after:rounded-full'
    : 'text-dim hover:text-primary hover:bg-white/8')

type Icon = (props: React.SVGProps<SVGSVGElement>) => React.ReactElement

const iconProps = {
  width: 17,
  height: 17,
  viewBox: '0 0 24 24',
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 1.6,
  strokeLinecap: 'round' as const,
  strokeLinejoin: 'round' as const,
}

const FilesIcon: Icon = (props) => (
  <svg {...iconProps} {...props}>
    <rect x="8" y="3" width="13" height="13" rx="2" />
    <rect x="3" y="8" width="13" height="13" rx="2" />
  </svg>
)

const SearchIcon: Icon = (props) => (
  <svg {...iconProps} {...props}>
    <circle cx="11" cy="11" r="6.5" />
    <path d="M20 20l-3.8-3.8" />
  </svg>
)

const BranchIcon: Icon = (props) => (
  <svg {...iconProps} {...props}>
    <circle cx="6" cy="6" r="2.4" />
    <circle cx="6" cy="18" r="2.4" />
    <circle cx="18" cy="8" r="2.4" />
    <path d="M6 8.4v7.2" />
    <path d="M18 10.4c0 3.4-2.8 4.4-6.2 4.6" />
  </svg>
)

const TerminalIcon: Icon = (props) => (
  <svg {...iconProps} {...props}>
    <circle cx="12" cy="12" r="9" />
    <path d="M10 8.5l4 3.5-4 3.5" />
  </svg>
)

const GearIcon: Icon = (props) => (
  <svg {...iconProps} {...props}>
    <circle cx="12" cy="12" r="3" />
    <path d="M12 2.8v2.4M12 18.8v2.4M2.8 12h2.4M18.8 12h2.4M5.5 5.5l1.7 1.7M16.8 16.8l1.7 1.7M18.5 5.5l-1.7 1.7M7.2 16.8l-1.7 1.7" />
  </svg>
)

const BotIcon: Icon = (props) => (
  <svg {...iconProps} {...props}>
    <rect x="4" y="8" width="16" height="11" rx="3" />
    <path d="M12 8V4.5" />
    <circle cx="12" cy="3.5" r="1.2" />
    <circle cx="9" cy="13.5" r="1" fill="currentColor" stroke="none" />
    <circle cx="15" cy="13.5" r="1" fill="currentColor" stroke="none" />
  </svg>
)

const GridIcon: Icon = (props) => (
  <svg {...iconProps} {...props} fill="currentColor" stroke="none">
    {[6, 12, 18].flatMap((y) =>
      [6, 12, 18].map((x) => <circle key={`${x}-${y}`} cx={x} cy={y} r="1.6" />),
    )}
  </svg>
)

const ActivityRail: React.FC<{
  ui: LayoutUI
  dockTab: 'agent' | 'git'
  onToggle: (key: 'sidebar' | 'rightDock' | 'terminal') => void
}> = ({ ui, dockTab, onToggle }) => {
  const { activeId } = useProjects()
  const { status } = useGit()
  const st = activeId ? status[activeId] : undefined
  const changed = st
    ? (st.staged?.length ?? 0) +
      (st.unstaged?.length ?? 0) +
      (st.untracked?.length ?? 0)
    : 0

  const openGitDock = () => {
    window.dispatchEvent(
      new CustomEvent('aide:set-dock-tab', { detail: { tab: 'git' } }),
    )
    if (!ui.rightDock) onToggle('rightDock')
  }

  const disabled = (label: string) => (
    <button
      className="no-drag w-7 h-7 rounded-md flex items-center justify-center text-dim/60 cursor-default"
      title={label}
      aria-label={label}
      disabled
    >
      {label === 'Settings — coming soon' ? <GearIcon /> : <GridIcon />}
    </button>
  )

  return (
    <div className="flex flex-col items-center gap-1.5 py-2 bg-panel w-11 shrink-0 border-r border-panel">
      <button
        className={railBtn(ui.sidebar)}
        title="Files ⌘B"
        aria-label="Toggle files sidebar"
        onClick={() => onToggle('sidebar')}
      >
        <FilesIcon />
      </button>
      <button
        className={railBtn(false)}
        title="Search ⌘P"
        aria-label="Open quick open"
        onClick={() => window.dispatchEvent(new Event('aide:quickopen'))}
      >
        <SearchIcon />
      </button>
      <button
        className={railBtn(ui.rightDock && dockTab === 'git')}
        title="Source Control ⌘D"
        aria-label="Toggle git dock"
        onClick={openGitDock}
      >
        <BranchIcon />
        {changed > 0 && (
          <span className="absolute -top-0.5 -right-1 rounded-full bg-[#3b5bfd] text-[9px] text-white px-1 h-3.5 min-w-3.5 flex items-center justify-center font-medium">
            {changed > 99 ? '99+' : changed}
          </span>
        )}
      </button>
      <button
        className={railBtn(ui.terminal)}
        title="Terminal ⌘J"
        aria-label="Toggle terminal"
        onClick={() => onToggle('terminal')}
      >
        <TerminalIcon />
      </button>
      <div className="mt-auto flex flex-col items-center gap-1.5">
        {disabled('Settings — coming soon')}
        {disabled('Extensions — coming soon')}
      </div>
    </div>
  )
}

const WorkspaceInner: React.FC = () => {
  const { ui, toggle, setSize } = useLayout()
  const [dockTab, setDockTab] = useState<'agent' | 'git'>('git')
  const { activeId } = useProjects()
  const { status } = useGit()
  const st = activeId ? status[activeId] : undefined
  const changed = st
    ? (st.staged?.length ?? 0) +
      (st.unstaged?.length ?? 0) +
      (st.untracked?.length ?? 0)
    : 0

  // Rail → dock tab coordination: the rail can force the dock onto a tab
  // (git badge click) without lifting tab state into the layout provider.
  useEffect(() => {
    const onTab = (e: Event) => {
      const tab = (e as CustomEvent<{ tab?: string }>).detail?.tab
      if (tab === 'agent' || tab === 'git') setDockTab(tab)
    }
    window.addEventListener('aide:set-dock-tab', onTab)
    return () => window.removeEventListener('aide:set-dock-tab', onTab)
  }, [])

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

  return (
    <div className="flex flex-col h-full bg-editor text-primary">
      <Titlebar />
      <div className="flex flex-1 min-h-0">
        {/* Activity rail */}
        <ActivityRail ui={ui} dockTab={dockTab} onToggle={toggle} />
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
                'relative flex items-center gap-1.5 px-3 py-2 ' +
                (dockTab === 'agent'
                  ? 'text-primary after:absolute after:left-2 after:right-2 after:bottom-0 after:h-[2px] after:bg-[var(--accent)] after:rounded-t'
                  : 'text-dim hover:text-primary')
              }
              onClick={() => setDockTab('agent')}
            >
              <BotIcon />
              Agent
            </button>
            <button
              className={
                'relative flex items-center gap-1.5 px-3 py-2 ' +
                (dockTab === 'git'
                  ? 'text-primary after:absolute after:left-2 after:right-2 after:bottom-0 after:h-[2px] after:bg-[var(--accent)] after:rounded-t'
                  : 'text-dim hover:text-primary')
              }
              onClick={() => setDockTab('git')}
            >
              <BranchIcon />
              Git
              {changed > 0 && (
                <span className="rounded-full bg-[#3b5bfd] text-[9px] text-white px-1 h-3.5 min-w-3.5 flex items-center justify-center font-medium">
                  {changed > 99 ? '99+' : changed}
                </span>
              )}
            </button>
            <button
              onClick={() => toggle('rightDock')}
              className="no-drag ml-auto mr-1 w-5 h-5 rounded flex items-center justify-center text-dim hover:text-primary hover:bg-[#373940]"
              title="Collapse dock (⌘D)"
              aria-label="Collapse right dock"
            >
              {'\u00bb'}
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

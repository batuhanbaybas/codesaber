import React from 'react'
import Titlebar from '../components/Titlebar'
import Sidebar from '../components/Sidebar'
import EditorTabs from '../components/EditorTabs'
import Editor from '../components/Editor'
import StatusBar from '../components/StatusBar'
import { ProjectsProvider } from '../state/projects'

const activityIcons = ['\u2318', '\u2192', '\u25a1', '\u21bb', '\u2699', '\u25be']

const Workspace: React.FC = () => {
  return (
    <ProjectsProvider>
      <div className="flex flex-col h-full bg-editor text-primary">
      <Titlebar />
      <div className="flex flex-1 min-h-0">
        {/* Activity rail */}
        <div className="flex flex-col items-center gap-1 py-2 bg-panel w-10 shrink-0 border-r border-panel">
          {activityIcons.map((icon, i) => (
            <button
              key={i}
              className={
                'no-drag w-8 h-8 rounded flex items-center justify-center text-sm ' +
                (i === 0 ? 'text-primary bg-[#1e1f22]' : 'text-dim hover:text-primary')
              }
            >
              {icon}
            </button>
          ))}
        </div>
        {/* Left sidebar */}
        <Sidebar />
        {/* Center editor area */}
        <div className="flex flex-col flex-1 min-w-0 bg-editor">
          <EditorTabs />
          <Editor />
        </div>
        {/* Right dock */}
        <div className="flex flex-col bg-panel w-64 shrink-0 border-l border-panel">
          <div className="flex border-b border-panel text-xs">
            <div className="px-3 py-2 border-l-2 border-transparent text-dim">Agent</div>
            <div className="px-3 py-2 border-l-2 border-transparent text-dim">Git</div>
          </div>
          <div className="flex-1 flex items-center justify-center text-xs text-dim">
            Empty placeholder
          </div>
        </div>
      </div>
      <StatusBar />
      {/* Bottom strip: terminal placeholder */}
      <div className="h-40 bg-panel border-t border-panel px-3 py-2 text-xs text-dim">
        Terminal
      </div>
      </div>
    </ProjectsProvider>
  )
}

export default Workspace

import React, { useEffect, useRef } from 'react'
import { EditorState, type Extension } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { basicSetup } from 'codemirror'
import { StreamLanguage } from '@codemirror/language'
import { javascript } from '@codemirror/lang-javascript'
import { css } from '@codemirror/lang-css'
import { html } from '@codemirror/lang-html'
import { json } from '@codemirror/lang-json'
import { go } from '@codemirror/legacy-modes/mode/go'
import { useProjects } from '../state/projects'
import { useTabs, type Tab } from '../state/tabs'

const languageFor = (path: string): Extension => {
  const name = path.slice(path.lastIndexOf('/') + 1).toLowerCase()
  if (name.endsWith('.go')) return StreamLanguage.define(go)
  if (name.endsWith('.tsx')) return javascript({ typescript: true, jsx: true })
  if (name.endsWith('.jsx')) return javascript({ jsx: true })
  if (name.endsWith('.ts')) return javascript({ typescript: true })
  if (name.endsWith('.js') || name.endsWith('.mjs') || name.endsWith('.cjs'))
    return javascript()
  if (name.endsWith('.css')) return css()
  if (name.endsWith('.html') || name.endsWith('.htm')) return html()
  if (name.endsWith('.json')) return json()
  return []
}

const theme = EditorView.theme(
  {
    '&': {
      height: '100%',
      fontSize: '13px',
      backgroundColor: 'var(--bg-editor)',
      color: 'var(--text-primary)',
    },
    '.cm-scroller': {
      fontFamily: '"SF Mono", Menlo, Monaco, monospace',
      lineHeight: '1.5',
    },
    '.cm-content': { caretColor: 'var(--accent)' },
    '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--accent)' },
    '.cm-gutters': {
      backgroundColor: 'var(--bg-editor)',
      color: 'var(--text-dim)',
      border: 'none',
      paddingRight: '8px',
    },
    '.cm-activeLine': { backgroundColor: 'rgba(255, 255, 255, 0.045)' },
    '.cm-activeLineGutter': {
      backgroundColor: 'rgba(255, 255, 255, 0.045)',
      color: 'var(--text-primary)',
    },
    '.cm-selectionBackground, &.cm-focused .cm-selectionBackground': {
      backgroundColor: 'rgba(74, 91, 252, 0.30)',
    },
    '.cm-selectionMatch': { backgroundColor: 'rgba(74, 91, 252, 0.18)' },
    '.cm-matchingBracket, &.cm-focused .cm-matchingBracket': {
      backgroundColor: 'rgba(74, 91, 252, 0.25)',
      outline: 'none',
      color: 'inherit',
    },
    '.cm-foldPlaceholder': {
      backgroundColor: 'var(--bg-panel)',
      border: '1px solid var(--bg-border)',
      color: 'var(--text-dim)',
    },
    '.cm-tooltip': {
      backgroundColor: 'var(--bg-panel)',
      border: '1px solid var(--bg-border)',
    },
    '.cm-panels': {
      backgroundColor: 'var(--bg-panel)',
      color: 'var(--text-primary)',
    },
  },
  { dark: true },
)

interface TabEditorProps {
  projectId: string
  tab: Tab
  active: boolean
  onSave?: (path: string, content: string) => void
}

const TabEditor: React.FC<TabEditorProps> = ({
  projectId,
  tab,
  active,
  onSave,
}) => {
  const { setDirty } = useTabs()
  const hostRef = useRef<HTMLDivElement | null>(null)
  const viewRef = useRef<EditorView | null>(null)
  const loadedContentRef = useRef<string | undefined>(undefined)
  const dirtyRef = useRef(false)
  const onSaveRef = useRef(onSave)
  onSaveRef.current = onSave

  useEffect(() => {
    if (!hostRef.current) return
    loadedContentRef.current = tab.dirContent
    dirtyRef.current = false
    const view = new EditorView({
      state: EditorState.create({
        doc: tab.dirContent ?? '',
        extensions: [
          basicSetup,
          keymap.of([
            {
              key: 'Mod-s',
              preventDefault: true,
              run: (v) => {
                onSaveRef.current?.(tab.path, v.state.doc.toString())
                return true
              },
            },
          ]),
          languageFor(tab.path),
          theme,
          EditorView.updateListener.of((u) => {
            if (!u.docChanged) return
            const dirty =
              u.state.doc.toString() !== (loadedContentRef.current ?? '')
            if (dirty !== dirtyRef.current) {
              dirtyRef.current = dirty
              setDirty(projectId, tab.path, dirty)
            }
          }),
        ],
      }),
      parent: hostRef.current,
    })
    viewRef.current = view
    return () => {
      view.destroy()
      viewRef.current = null
    }
    // Recreate per open tab; content sync handled below via loadedContentRef.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab.path, projectId, setDirty])

  useEffect(() => {
    const view = viewRef.current
    if (!view) return
    if (
      tab.dirContent !== undefined &&
      tab.dirContent !== loadedContentRef.current
    ) {
      loadedContentRef.current = tab.dirContent
      view.dispatch({
        changes: {
          from: 0,
          to: view.state.doc.length,
          insert: tab.dirContent,
        },
      })
      if (dirtyRef.current) {
        dirtyRef.current = false
        setDirty(projectId, tab.path, false)
      }
    }
  }, [tab.dirContent, tab.path, projectId, setDirty])

  useEffect(() => {
    if (active) viewRef.current?.requestMeasure()
  }, [active])

  return (
    <div
      ref={hostRef}
      className="absolute inset-0 overflow-hidden"
      style={{ display: active ? 'block' : 'none' }}
    />
  )
}

const Editor: React.FC<{ onSave?: (path: string, content: string) => void }> = ({
  onSave,
}) => {
  const { activeId } = useProjects()
  const { tabsByProject } = useTabs()
  const state = activeId ? tabsByProject[activeId] : undefined

  if (!activeId || !state || !state.active) {
    return (
      <div className="flex-1 flex items-center justify-center bg-editor min-h-0">
        <div className="w-3/4 h-1/2 border-2 border-dashed rounded-lg flex items-center justify-center text-dim">
          Editor surface {'\u2014'} open a file to start editing
        </div>
      </div>
    )
  }

  return (
    <div className="relative flex-1 min-h-0 bg-editor">
      {state.open.map((t) => (
        <TabEditor
          key={t.path}
          projectId={activeId}
          tab={t}
          active={t.path === state.active}
          onSave={onSave}
        />
      ))}
    </div>
  )
}

export default Editor

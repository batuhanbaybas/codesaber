import React, { useEffect, useRef } from 'react'
import {
  EditorState,
  StateEffect,
  StateField,
  type Extension,
} from '@codemirror/state'
import {
  EditorView,
  keymap,
  gutter,
  GutterMarker,
  hoverTooltip,
  showTooltip,
  ViewPlugin,
  type Tooltip,
} from '@codemirror/view'
import { basicSetup } from 'codemirror'
import { StreamLanguage } from '@codemirror/language'
import { javascript } from '@codemirror/lang-javascript'
import { css } from '@codemirror/lang-css'
import { html } from '@codemirror/lang-html'
import { json } from '@codemirror/lang-json'
import { go } from '@codemirror/legacy-modes/mode/go'
import { Events } from '@wailsio/runtime'
import { useProjects } from '../state/projects'
import { useTabs, type Tab } from '../state/tabs'
import DiffViewer from './DiffViewer'
import * as App from '../../bindings/aide/backend/app'
import * as LSP from '../lsp'

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

const setDiagEffect = StateEffect.define<LSP.Diagnostic[] | null>()
const activeTipEffect = StateEffect.define<Tooltip | null>()

// diagsField keeps per-line diagnostics (line index → list) for the tab.
const diagsField = StateField.define<Map<number, LSP.Diagnostic[]>>({
  create: () => new Map(),
  update(lineDiags, tr) {
    let next = lineDiags
    for (const e of tr.effects) {
      if (e.is(setDiagEffect)) {
        next = new Map()
        if (e.value) {
          for (const d of e.value) {
            const arr = next.get(d.range.start.line) ?? []
            arr.push(d)
            next.set(d.range.start.line, arr)
          }
        }
      }
    }
    return next
  },
})

const tipField = StateField.define<Tooltip | null>({
  create: () => null,
  update(tip, tr) {
    for (const e of tr.effects) {
      if (e.is(activeTipEffect)) tip = e.value
      if (e.is(setDiagEffect)) tip = null
    }
    return tip
  },
  provide: (f) => showTooltip.from(f, (tip) => tip),
})

const sevColor = (severity?: number): string =>
  severity === 1
    ? '#c75454'
    : severity === 2
      ? '#bb9a3c'
      : '#4a91e2'

class DiagMarker extends GutterMarker {
  constructor(private color: string) {
    super()
  }
  toDOM(): Node {
    const dot = document.createElement('div')
    dot.className = 'cm-lsp-diag-dot'
    dot.style.background = this.color
    return dot
  }
}

const diagMarkers = new Map<string, DiagMarker>()
const markerFor = (severity?: number) => {
  const color = sevColor(severity)
  let m = diagMarkers.get(color)
  if (!m) {
    m = new DiagMarker(color)
    diagMarkers.set(color, m)
  }
  return m
}

const diagGutter = gutter({
  class: 'cm-lsp-diag-gutter',
  lineMarker: (view, line) => {
    const ds = view.state.field(diagsField).get(
      view.state.doc.lineAt(line.from).number - 1,
    )
    return ds?.length ? markerFor(ds[0].severity) : null
  },
  lineMarkerChange: () => true,
  domEventHandlers: {
    mousedown(view, line) {
      const ds = view.state.field(diagsField).get(
        view.state.doc.lineAt(line.from).number - 1,
      )
      if (!ds?.length) return false
      const dom = document.createElement('div')
      dom.className = 'cm-lsp-diag-tip'
      for (const d of ds) {
        const row = document.createElement('div')
        row.style.display = 'flex'
        row.style.gap = '6px'
        const dot = document.createElement('span')
        dot.className = 'cm-lsp-diag-dot'
        dot.style.background = sevColor(d.severity)
        dot.style.display = 'inline-block'
        dot.style.width = '7px'
        dot.style.height = '7px'
        dot.style.borderRadius = '50%'
        dot.style.marginTop = '5px'
        const text = document.createElement('span')
        text.textContent = d.message
        row.appendChild(dot)
        row.appendChild(text)
        dom.appendChild(row)
      }
      view.dispatch({
        effects: activeTipEffect.of({
          pos: line.from,
          create: () => ({ dom }),
        }),
      })
      return true
    },
  },
})

// modTracker tracks whether Mod is currently held (keydown/keyup on the
// editor DOM) so hover and goto-definition only react with Mod.
let modHeld = false
const modTracker = ViewPlugin.fromClass(
  class {
    private view: EditorView
    private onKey = (e: KeyboardEvent): void => {
      modHeld = e.metaKey || e.ctrlKey
    }
    constructor(view: EditorView) {
      this.view = view
      view.dom.addEventListener('keydown', this.onKey)
      view.dom.addEventListener('keyup', this.onKey)
    }
    destroy(): void {
      this.view.dom.removeEventListener('keydown', this.onKey)
      this.view.dom.removeEventListener('keyup', this.onKey)
    }
  },
)

// goHover turns a Mod-hover into a gopls hover tooltip (plain text).
const goHover = (projectId: string, path: string): Extension =>
  hoverTooltip(
    async (view, pos) => {
      if (!modHeld) return null
      const line = view.state.doc.lineAt(pos)
      const h = await LSP.hover(
        projectId,
        path,
        line.number - 1,
        pos - line.from,
      ).catch(() => null)
      const text = LSP.hoverText(h)
      if (!text) return null
      const dom = document.createElement('div')
      dom.className = 'cm-lsp-hover'
      dom.textContent = text
      return { pos, create: () => ({ dom }) }
    },
    { hideOnChange: true, hoverTime: 250 },
  )

// gotoOnModClick wires Mod+click → LSPDefinition → openFile(reveal).
const gotoOnModClick = (
  projectId: string,
  path: string,
  openFile: (p: string, path: string, opts?: { reveal?: { line: number; character: number } }) => Promise<void>,
): Extension =>
  EditorView.domEventHandlers({
    mousedown(event, view) {
      const mod = event.metaKey || event.ctrlKey
      if (!mod) {
        // clicking anywhere without Mod dismisses an open diag tooltip
        if (view.state.field(tipField, false))
          view.dispatch({ effects: activeTipEffect.of(null) })
        return false
      }
      const pos = view.posAtCoords({ x: event.clientX, y: event.clientY })
      if (pos == null) return false
      event.preventDefault()
      const line = view.state.doc.lineAt(pos)
      LSP.definition(projectId, path, line.number - 1, pos - line.from)
        .then((locs) => {
          const target = locs?.[0]
          if (!target || !target.uri) return
          return openFile(projectId, target.uri, {
            reveal: {
              line: target.range.start.line,
              character: target.range.start.character,
            },
          })
        })
        .catch(() => {})
      return true
    },
  })

const lspTheme = EditorView.theme({
  '.cm-lsp-diag-gutter': { width: '10px' },
  '.cm-lsp-diag-dot': {
    width: '7px',
    height: '7px',
    borderRadius: '50%',
    margin: '0 auto',
    display: 'inline-block',
  },
  '.cm-lsp-diag-tip, .cm-tooltip-hover': {
    backgroundColor: 'var(--bg-panel)',
    border: '1px solid var(--bg-border)',
    padding: '4px 8px',
    color: 'var(--text-primary)',
    fontSize: '12px',
    maxWidth: '420px',
    whiteSpace: 'pre-wrap',
  },
})

const TabEditor: React.FC<{
  projectId: string
  tab: Tab
  active: boolean
}> = ({ projectId, tab, active }) => {
  const { setDirty, save, reload, keepMine, openFile, consumeReveal } =
    useTabs()
  const hostRef = useRef<HTMLDivElement | null>(null)
  const viewRef = useRef<EditorView | null>(null)
  const loadedContentRef = useRef<string | undefined>(undefined)
  const dirtyRef = useRef(false)
  const openedRef = useRef(false)
  const onSaveRef = useRef(save)
  const onOpenFileRef = useRef(openFile)
  onSaveRef.current = save
  onOpenFileRef.current = openFile
  const isGo = tab.path.endsWith('.go')

  useEffect(() => {
    if (!hostRef.current) return
    loadedContentRef.current = tab.dirContent
    dirtyRef.current = false
    openedRef.current = false
    const view = new EditorView({
      state: EditorState.create({
        doc: tab.dirContent ?? '',
        extensions: [
          // diagnostics gutter goes before basicSetup so it renders to the
          // left of the line numbers
          diagsField,
          tipField,
          diagGutter,
          basicSetup,
          theme,
          lspTheme,
          keymap.of([
            {
              key: 'Mod-s',
              preventDefault: true,
              run: (v) => {
                if (isGo) {
                  LSP.flushDidChange(projectId, tab.path)
                  onSaveRef.current?.(projectId, tab.path, v.state.doc.toString())
                  LSP.didSave(projectId, tab.path, v.state.doc.toString()).catch(
                    () => {},
                  )
                } else {
                  onSaveRef.current?.(projectId, tab.path, v.state.doc.toString())
                }
                return true
              },
            },
          ]),
          languageFor(tab.path),
          modTracker,
          ...(isGo ? [goHover(projectId, tab.path), gotoOnModClick(projectId, tab.path, onOpenFileRef.current)] : []),
          EditorView.updateListener.of((u) => {
            if (!u.docChanged) return
            if (isGo) {
              LSP.didChange(projectId, tab.path, u.state.doc.toString())
            }
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
      if (openedRef.current) {
        LSP.flushDidChange(projectId, tab.path)
        LSP.didClose(projectId, tab.path)
      }
      view.destroy()
      viewRef.current = null
    }
    // Recreate per open tab; content sync handled below via loadedContentRef.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab.path, projectId, setDirty])

  // didOpen once per tab once content exists (gopls must see full text).
  useEffect(() => {
    if (!isGo || openedRef.current || tab.dirContent === undefined) return
    LSP.ensure(projectId).catch(() => {})
    openedRef.current = true
    LSP.didOpen(projectId, tab.path, tab.dirContent).catch(() => {})
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isGo, tab.dirContent])

  // lsp.diag events → per-tab per-line diagnostic markers.
  useEffect(() => {
    if (!isGo) return
    const off = Events.On('lsp.diag', (ev: any) => {
      const d = (ev.data ?? {}) as LSP.LSPDiagEvent
      if (!d || d.projectId !== projectId || d.path !== tab.path) return
      viewRef.current?.dispatch({
        effects: setDiagEffect.of((d.diagnostics ?? []) as LSP.Diagnostic[]),
      })
    })
    return () => off()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isGo, projectId, tab.path])

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
    const view = viewRef.current
    if (!view || !tab.reveal) return
    // wait until content has actually landed so line numbers are valid
    if (view.state.doc.lines < 2 && tab.dirContent !== '') {
      if (tab.dirContent === undefined && view.state.doc.length === 0) return
    }
    const lineNo = Math.min(tab.reveal.line + 1, view.state.doc.lines)
    const line = view.state.doc.line(lineNo)
    const ch = Math.min(tab.reveal.character, line.length)
    view.dispatch({
      selection: { anchor: line.from + ch, head: line.from + ch },
      effects: EditorView.scrollIntoView(line.from + ch, { y: 'center' }),
    })
    view.focus()
    consumeReveal(projectId, tab.path)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab.reveal, tab.dirContent])

  useEffect(() => {
    if (active) viewRef.current?.requestMeasure()
  }, [active])

  return (
    <div
      className="absolute inset-0 flex flex-col"
      style={{ display: active ? 'flex' : 'none' }}
    >
      {tab.staleExternally && (
        <div className="flex items-center gap-3 px-3 py-1 text-xs bg-[#3b2f1e] border-b border-[#b58900]/40 text-[#e6c07b] shrink-0">
          <span>File changed on disk</span>
          <button
            className="no-drag px-2 py-0.5 rounded border border-[#e6c07b]/50 hover:bg-[#e6c07b]/15"
            onClick={() =>
              App.ReadFile(tab.path)
                .then((c) => reload(projectId, tab.path, c))
                .catch(() => {})
            }
          >
            Reload
          </button>
          <button
            className="no-drag px-2 py-0.5 rounded border border-[#e6c07b]/50 hover:bg-[#e6c07b]/15"
            onClick={() => keepMine(projectId, tab.path)}
          >
            Keep mine
          </button>
        </div>
      )}
      <div
        ref={hostRef}
        className="relative flex-1 min-h-0 overflow-hidden"
      />
    </div>
  )
}

const Editor: React.FC = () => {
  const { activeId } = useProjects()
  const { tabsByProject, saveError } = useTabs()
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
    <div className="relative flex-1 min-h-0 bg-editor flex flex-col">
      {saveError && (
        <div className="px-3 py-1 text-xs bg-[#5a1d1d] border-b border-[#ff6b6b]/40 text-[#ff9999] shrink-0">
          {saveError}
        </div>
      )}
      <div className="relative flex-1 min-h-0">
        {state.open.map((t) =>
          t.kind === 'diff' ? (
            <DiffViewer
              key={t.path}
              projectId={activeId}
              tab={t}
              active={t.path === state.active}
            />
          ) : (
            <TabEditor
              key={t.path}
              projectId={activeId}
              tab={t}
              active={t.path === state.active}
            />
          ),
        )}
      </div>
    </div>
  )
}

export default Editor

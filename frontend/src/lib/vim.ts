// Vim mode for the editor via @replit/codemirror-vim; `status: true` renders
// the --NORMAL--/--INSERT-- panel at the bottom (themed below).
import { EditorView } from '@codemirror/view'
import type { Extension } from '@codemirror/state'
import { vim } from '@replit/codemirror-vim'

const vimPanelTheme = EditorView.theme(
  {
    '.cm-vim-panel': {
      display: 'flex',
      alignItems: 'center',
      gap: '8px',
      padding: '2px 10px',
      fontFamily: '"SF Mono", Menlo, Monaco, monospace',
      fontSize: '11px',
      backgroundColor: 'var(--bg-panel)',
      color: 'var(--text-dim)',
      borderTop: '1px solid var(--bg-border)',
    },
    '.cm-vim-panel input': {
      flex: '1 1 auto',
      minWidth: '0',
      fontFamily: 'inherit',
      fontSize: 'inherit',
      backgroundColor: 'transparent',
      color: 'var(--text-primary)',
      border: 'none',
      outline: 'none',
    },
  },
  { dark: true },
)

export const vimExtension = (): Extension => [vim({ status: true }), vimPanelTheme]

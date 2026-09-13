import React, { useMemo } from 'react'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { useProjects } from '../state/projects'
import {
  useTabs,
  mdPreviewTabPath,
} from '../state/tabs'

// Markdown preview: replaces the CodeMirror surface for .md/.markdown tabs.
// marked renders HTML which is then sanitized with DOMPurify before the
// result goes anywhere near dangerouslySetInnerHTML.

marked.setOptions({ gfm: true, breaks: false })

const EyeIcon: React.FC<{ open?: boolean }> = ({ open }) => (
  <svg
    width="13"
    height="13"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.7"
    strokeLinecap="round"
    strokeLinejoin="round"
  >
    {open ? (
      <>
        <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94" />
        <path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19" />
        <path d="M14.12 14.12A3 3 0 1 1 9.88 9.88" />
        <line x1="1" y1="1" x2="23" y2="23" />
      </>
    ) : (
      <>
        <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
        <circle cx="12" cy="12" r="3" />
      </>
    )}
  </svg>
)

export const MdPreviewToggle: React.FC<{ path: string }> = ({ path }) => {
  const { activeId } = useProjects()
  const { tabsByProject, openMdPreviewTab, closeMdPreviewTab } = useTabs()
  const on =
    !!activeId &&
    !!tabsByProject[activeId]?.open.some(
      (t) => t.path === mdPreviewTabPath(path),
    )
  return (
    <button
      className={
        'no-drag shrink-0 flex items-center justify-center px-2.5 h-full ' +
        (on
          ? 'text-[var(--accent)] hover:text-primary'
          : 'text-dim hover:text-primary')
      }
      title={on ? 'Close preview' : 'Preview Markdown'}
      aria-label={on ? 'Close markdown preview' : 'Show markdown preview'}
      aria-pressed={on}
      onClick={() => {
        if (!activeId) return
        if (on) closeMdPreviewTab(activeId, path)
        else openMdPreviewTab(activeId, path)
      }}
    >
      <EyeIcon open={on} />
    </button>
  )
}

const MarkdownPreview: React.FC<{ content: string }> = ({ content }) => {
  const html = useMemo(
    () => DOMPurify.sanitize(marked.parse(content, { async: false }) as string),
    [content],
  )
  return <div className="md-preview" dangerouslySetInnerHTML={{ __html: html }} />
}

export default MarkdownPreview

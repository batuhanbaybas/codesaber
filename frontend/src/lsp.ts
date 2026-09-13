import * as App from '../bindings/codesaber/backend/app'
import type { Location, DocumentSymbol, Hover } from '../bindings/codesaber/backend/lsp/models'

// Diagnostic mirrors the backend lsp.Diagnostic payload in "lsp.diag" events.
export interface Diagnostic {
  range: {
    start: { line: number; character: number }
    end: { line: number; character: number }
  }
  severity?: number
  message: string
  source?: string
}

export interface LSPDiagEvent {
  projectId: string
  path: string
  diagnostics: Diagnostic[]
}

export interface LSPStateEvent {
  projectId: string
  running: boolean
  reason?: string
}

// Version bookkeeping and 300ms didChange debounce timers are keyed on
// projectId\0path; full-content sync means no range diffing is needed.
// coldQueue holds didChange calls made while the project's server was still
// starting (backend answered "no running server"); the latest content per
// path is flushed on the next successful didOpen/ensure.
const versions = new Map<string, number>()
const changeTimers = new Map<string, number>()
const pendingContent = new Map<string, string>()
const pendingProject = new Map<string, string>()
const pendingPath = new Map<string, string>()
const coldQueue = new Map<string, {
  projectId: string
  path: string
  version: number
  content: string
}>()

const key = (projectId: string, path: string) => `${projectId}\0${path}`

const nextVersion = (k: string) => {
  const v = (versions.get(k) ?? 0) + 1
  versions.set(k, v)
  return v
}

// sendDidChange delivers a full-content didChange, queueing it when the
// server is not running yet (cold start) so the edit is not lost.
const sendDidChange = (
  projectId: string,
  path: string,
  version: number,
  content: string,
): void => {
  App.LSPDidChange(projectId, path, version, content).catch((e: unknown) => {
    if (String(e).includes('no running server')) {
      coldQueue.set(key(projectId, path), { projectId, path, version, content })
    }
  })
}

// flushColdQueue drains queued cold-start edits; latest per path wins.
// Called after a successful didOpen so the server starts from current text.
const flushColdQueue = (projectId: string, path: string): void => {
  const k = key(projectId, path)
  const q = coldQueue.get(k)
  if (!q) return
  coldQueue.delete(k)
  sendDidChange(q.projectId, q.path, q.version, q.content)
}

export const didOpen = (
  projectId: string,
  path: string,
  content: string,
): Promise<void> => {
  const v = nextVersion(key(projectId, path))
  return App.LSPDidOpen(projectId, path, v, content).then(() =>
    flushColdQueue(projectId, path),
  )
}

export const ensure = async (projectId: string): Promise<void> => {
  await App.LSPEnsure(projectId)
  for (const q of [...coldQueue.values()]) {
    if (q.projectId === projectId) flushColdQueue(projectId, q.path)
  }
}

export const didChange = (
  projectId: string,
  path: string,
  content: string,
): void => {
  const k = key(projectId, path)
  const version = nextVersion(k)
  pendingContent.set(k, content)
  pendingProject.set(k, projectId)
  pendingPath.set(k, path)
  if (changeTimers.has(k)) return
  changeTimers.set(
    k,
    window.setTimeout(() => {
      changeTimers.delete(k)
      const v = versions.get(k) ?? version
      const proj = pendingProject.get(k)
      const ppath = pendingPath.get(k)
      const text = pendingContent.get(k)
      if (!proj || !ppath) return
      sendDidChange(proj, ppath, v, text ?? '')
    }, 300),
  )
}

// flushDidChange sends any pending didChange immediately (before save/closing
// so the server sees the final content).
export const flushDidChange = (projectId: string, path: string): void => {
  const k = key(projectId, path)
  const timer = changeTimers.get(k)
  if (timer === undefined) return
  window.clearTimeout(timer)
  changeTimers.delete(k)
  const v = versions.get(k)
  const proj = pendingProject.get(k)
  const ppath = pendingPath.get(k)
  const text = pendingContent.get(k)
  if (v === undefined || !proj || !ppath || text === undefined) return
  sendDidChange(proj, ppath, v, text)
}

export const didSave = (projectId: string, path: string, content: string) =>
  App.LSPDidSave(projectId, path, content)

export const didClose = (projectId: string, path: string) => {
  const k = key(projectId, path)
  const timer = changeTimers.get(k)
  if (timer !== undefined) window.clearTimeout(timer)
  versions.delete(k)
  changeTimers.delete(k)
  pendingContent.delete(k)
  pendingProject.delete(k)
  pendingPath.delete(k)
  coldQueue.delete(k)
  return App.LSPDidClose(projectId, path).catch(() => {})
}

export const definition = (
  projectId: string,
  path: string,
  line: number,
  character: number,
): Promise<Location[] | null> =>
  App.LSPDefinition(projectId, path, line, character)

export const hover = (
  projectId: string,
  path: string,
  line: number,
  character: number,
): Promise<Hover | null> => App.LSPHover(projectId, path, line, character)

export const symbols = (
  projectId: string,
  path: string,
): Promise<DocumentSymbol[] | null> => App.LSPSymbols(projectId, path)

// hoverText flattens the hover result's raw JSON contents into plain text for
// a tooltip: MarkupContent → value, string → itself, array → joined lines.
export const hoverText = (h: Hover | null): string => {
  if (!h) return ''
  try {
    let c: unknown = JSON.parse(h.contents as unknown as string)
    const one = (v: unknown): string => {
      if (typeof v === 'string') return v
      if (Array.isArray(v)) return v.map(one).join('\n')
      if (v && typeof v === 'object') {
        const o = v as { value?: unknown }
        if ('value' in o) return one(o.value)
      }
      return ''
    }
    return one(c).trim()
  } catch {
    return String(h.contents ?? '').trim()
  }
}

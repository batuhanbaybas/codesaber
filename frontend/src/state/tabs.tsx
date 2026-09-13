import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from 'react'
import { Events } from '@wailsio/runtime'
import * as App from '../../bindings/codesaber/backend/app'

export interface Tab {
  path: string
  title: string
  dirContent?: string
  dirty: boolean
  staleExternally?: boolean
  kind?: 'file' | 'diff' | 'md-preview'
  diffStaged?: boolean
  reveal?: { line: number; character: number }
}

export interface ProjectTabs {
  open: Tab[]
  active: string | null
}

interface TabsContextValue {
  tabsByProject: Record<string, ProjectTabs>
  saveError: string | null
  openFile: (
    projectId: string,
    path: string,
    opts?: { reveal?: { line: number; character: number } },
  ) => Promise<void>
  consumeReveal: (projectId: string, path: string) => void
  close: (projectId: string, path: string) => void
  setActive: (projectId: string, path: string) => void
  setDirty: (projectId: string, path: string, dirty: boolean) => void
  save: (projectId: string, path: string, content: string) => Promise<void>
  reload: (projectId: string, path: string, content: string) => void
  keepMine: (projectId: string, path: string) => void
  openDiffTab: (projectId: string, path: string, staged: boolean) => void
  openMdPreviewTab: (projectId: string, path: string) => void
  closeMdPreviewTab: (projectId: string, path: string) => void
}

const TabsContext = createContext<TabsContextValue | null>(null)

const titleOf = (path: string) => path.slice(path.lastIndexOf('/') + 1) || path

// diffTabPath encodes a diff tab's identity in its key so diff and file tabs
// can coexist in the strip without colliding on path.
export const diffTabPath = (path: string, staged: boolean) =>
  `\u0394:${staged ? 's' : 'u'}:${path}`

// parseDiffTabPath reverses diffTabPath (title prefix \u0394, then staged
// flag, then the real workspace-relative path).
export const parseDiffTabPath = (
  key: string,
): { path: string; staged: boolean } | null => {
  if (!key.startsWith('\u0394:')) return null
  const rest = key.slice(2)
  const colon = rest.indexOf(':')
  if (colon === -1) return null
  return { path: rest.slice(colon + 1), staged: rest.slice(0, colon) === 's' }
}

// mdPreviewTabPath encodes a markdown preview tab's identity in its key so
// preview and source tabs coexist in the strip without colliding on path.
export const mdPreviewTabPath = (path: string) => `\u25C7:${path}`

// parseMdPreviewTabPath reverses mdPreviewTabPath (prefix \u25C7, then the
// real workspace-relative path).
export const parseMdPreviewTabPath = (key: string): string | null =>
  key.startsWith('\u25C7:') ? key.slice(2) : null

// Window during which an fs.change echo for a path we just saved is ignored.
const saveEchoWindowMs = 2000

export const TabsProvider: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  const [tabsByProject, setTabsByProject] = useState<
    Record<string, ProjectTabs>
  >({})
  const [saveError, setSaveError] = useState<string | null>(null)
  const tabsRef = useRef(tabsByProject)
  tabsRef.current = tabsByProject
  const lastSaveRef = useRef<Record<string, number>>({})
  const errorTimerRef = useRef<number | undefined>(undefined)

  const mutateTab = useCallback(
    (
      projectId: string,
      path: string,
      patch: (t: Tab) => Partial<Tab>,
    ) => {
      setTabsByProject((prev) => {
        const st = prev[projectId]
        if (!st) return prev
        if (!st.open.some((t) => t.path === path)) return prev
        return {
          ...prev,
          [projectId]: {
            ...st,
            open: st.open.map((t) =>
              t.path === path ? { ...t, ...patch(t) } : t,
            ),
          },
        }
      })
    },
    [],
  )

  const openFile = useCallback(
    async (
      projectId: string,
      path: string,
      opts?: { reveal?: { line: number; character: number } },
    ) => {
      const existing = tabsRef.current[projectId]?.open.find(
        (t) => t.path === path,
      )
      const needFetch = !existing || existing.dirContent == null
      setTabsByProject((prev) => {
        const st = prev[projectId] ?? { open: [], active: null }
        if (st.open.some((t) => t.path === path)) {
          return {
            ...prev,
            [projectId]: { ...st, active: path, open: st.open.map((t) => t.path === path ? { ...t, reveal: opts?.reveal } : t) },
          }
        }
        const tab: Tab = {
          path,
          title: titleOf(path),
          dirty: false,
          reveal: opts?.reveal,
        }
        return {
          ...prev,
          [projectId]: { open: [...st.open, tab], active: path },
        }
      })
      if (!needFetch) return
      try {
        const content = await App.ReadFile(path)
        setTabsByProject((prev) => {
          const st = prev[projectId]
          if (!st) return prev
          return {
            ...prev,
            [projectId]: {
              ...st,
              open: st.open.map((t) =>
                t.path === path ? { ...t, dirContent: content } : t,
              ),
            },
          }
        })
      } catch {
        // leave tab without content; Task 10 editor will surface errors
      }
    },
    [],
  )

  const close = useCallback((projectId: string, path: string) => {
    setTabsByProject((prev) => {
      const st = prev[projectId]
      if (!st) return prev
      const idx = st.open.findIndex((t) => t.path === path)
      if (idx === -1) return prev
      const open = st.open.filter((t) => t.path !== path)
      let active = st.active
      if (active === path) {
        active = open[Math.min(idx, open.length - 1)]?.path ?? null
      }
      return { ...prev, [projectId]: { open, active } }
    })
  }, [])

  const consumeReveal = useCallback(
    (projectId: string, path: string) => {
      mutateTab(projectId, path, () => ({ reveal: undefined }))
    },
    [mutateTab],
  )

  const setActive = useCallback((projectId: string, path: string) => {
    setTabsByProject((prev) => {
      const st = prev[projectId]
      if (!st || !st.open.some((t) => t.path === path)) return prev
      return { ...prev, [projectId]: { ...st, active: path } }
    })
  }, [])

  const setDirty = useCallback(
    (projectId: string, path: string, dirty: boolean) => {
      setTabsByProject((prev) => {
        const st = prev[projectId]
        if (!st) return prev
        return {
          ...prev,
          [projectId]: {
            ...st,
            open: st.open.map((t) =>
              t.path === path ? { ...t, dirty } : t,
            ),
          },
        }
      })
    },
    [],
  )

  const save = useCallback(
    async (projectId: string, path: string, content: string) => {
      try {
        await App.SaveFile(path, content)
        lastSaveRef.current[`${projectId}\0${path}`] = Date.now()
        mutateTab(projectId, path, () => ({
          dirty: false,
          staleExternally: false,
          dirContent: content,
        }))
        setSaveError(null)
      } catch (e) {
        setSaveError(`Save failed: ${String(e)}`)
        window.clearTimeout(errorTimerRef.current)
        errorTimerRef.current = window.setTimeout(
          () => setSaveError(null),
          4000,
        )
      }
    },
    [mutateTab],
  )

  const reload = useCallback(
    (projectId: string, path: string, content: string) => {
      mutateTab(projectId, path, () => ({
        dirContent: content,
        dirty: false,
        staleExternally: false,
      }))
    },
    [mutateTab],
  )

  const keepMine = useCallback(
    (projectId: string, path: string) => {
      mutateTab(projectId, path, () => ({ staleExternally: false }))
    },
    [mutateTab],
  )

  // Diff tabs are never dirty: open is idempotent (activates an existing
  // diff tab); no file content is fetched here — the diff viewer pulls the
  // patch from the git provider by key.
  const openDiffTab = useCallback(
    (projectId: string, path: string, staged: boolean) => {
      const key = diffTabPath(path, staged)
      setTabsByProject((prev) => {
        const st = prev[projectId] ?? { open: [], active: null }
        if (st.open.some((t) => t.path === key)) {
          return { ...prev, [projectId]: { ...st, active: key } }
        }
        const tab: Tab = {
          path: key,
          title: `\u0394 ${titleOf(path)}`,
          dirty: false,
          kind: 'diff',
          diffStaged: staged,
        }
        return {
          ...prev,
          [projectId]: { open: [...st.open, tab], active: key },
        }
      })
    },
    [],
  )

  // Markdown preview tabs mirror diff tabs: never dirty, open is idempotent
  // (activates an existing preview tab), content fetched once on creation.
  const openMdPreviewTab = useCallback(
    (projectId: string, path: string) => {
      const key = mdPreviewTabPath(path)
      const existing = tabsRef.current[projectId]?.open.find(
        (t) => t.path === key,
      )
      const needFetch = !existing || existing.dirContent == null
      setTabsByProject((prev) => {
        const st = prev[projectId] ?? { open: [], active: null }
        if (st.open.some((t) => t.path === key)) {
          return { ...prev, [projectId]: { ...st, active: key } }
        }
        const tab: Tab = {
          path: key,
          title: titleOf(path),
          dirty: false,
          kind: 'md-preview',
        }
        return {
          ...prev,
          [projectId]: { open: [...st.open, tab], active: key },
        }
      })
      if (!needFetch) return
      App.ReadFile(path)
        .then((content) => {
          setTabsByProject((prev) => {
            const st = prev[projectId]
            if (!st) return prev
            return {
              ...prev,
              [projectId]: {
                ...st,
                open: st.open.map((t) =>
                  t.path === key ? { ...t, dirContent: content } : t,
                ),
              },
            }
          })
        })
        .catch(() => {})
    },
    [],
  )

  // Closing a preview tab returns focus to the source tab when the preview
  // was active, mirroring how the eye toggle reads as "leave preview".
  const closeMdPreviewTab = useCallback((projectId: string, path: string) => {
    const key = mdPreviewTabPath(path)
    setTabsByProject((prev) => {
      const st = prev[projectId]
      if (!st || !st.open.some((t) => t.path === key)) return prev
      const idx = st.open.findIndex((t) => t.path === key)
      const open = st.open.filter((t) => t.path !== key)
      let active = st.active
      if (active === key) {
        active = st.open.some((t) => t.path === path)
          ? path
          : (open[Math.min(idx, open.length - 1)]?.path ?? null)
      }
      return { ...prev, [projectId]: { open, active } }
    })
  }, [])

  // fs.change reconcile: auto-reload non-dirty tabs, flag dirty tabs; the
  // editor surface renders the banner. remove/rename silently closes
  // non-dirty tabs. Immediately-after-save echoes are ignored so our own
  // save never triggers a reconcile against stale state.
  useEffect(() => {
    const off = Events.On('fs.change', (ev: any) => {
      const { projectId, path, op } = (ev.data ?? {}) as {
        projectId?: string
        path?: string
        op?: string
      }
      if (!projectId || !path) return
      const tab = tabsRef.current[projectId]?.open.find(
        (t) => t.path === path,
      )
      const previewKey = mdPreviewTabPath(path)
      const previewTab = tabsRef.current[projectId]?.open.find(
        (t) => t.path === previewKey,
      )
      if (op === 'remove' || op === 'rename') {
        if (previewTab) close(projectId, previewKey)
        if (!tab) return
        if (!tab.dirty) close(projectId, path)
        return
      }
      const savedAt = lastSaveRef.current[`${projectId}\0${path}`]
      if (savedAt && Date.now() - savedAt < saveEchoWindowMs) return
      if (tab?.dirty) {
        mutateTab(projectId, path, () => ({ staleExternally: true }))
        return
      }
      App.ReadFile(path)
        .then((content) => {
          if (tab) reload(projectId, path, content)
          if (previewTab) reload(projectId, previewKey, content)
        })
        .catch(() => {})
    })
    return () => off()
  }, [close, mutateTab, reload])

  return (
    <TabsContext.Provider
      value={{
        tabsByProject,
        saveError,
        openFile,
        close,
        consumeReveal,
        setActive,
        setDirty,
        save,
        reload,
        keepMine,
        openDiffTab,
        openMdPreviewTab,
        closeMdPreviewTab,
      }}
    >
      {children}
    </TabsContext.Provider>
  )
}

export const useTabs = (): TabsContextValue => {
  const ctx = useContext(TabsContext)
  if (!ctx) throw new Error('useTabs must be used within TabsProvider')
  return ctx
}

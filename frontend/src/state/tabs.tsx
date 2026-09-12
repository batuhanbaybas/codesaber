import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from 'react'
import { Events } from '@wailsio/runtime'
import * as App from '../../bindings/aide/backend/app'

export interface Tab {
  path: string
  title: string
  dirContent?: string
  dirty: boolean
  staleExternally?: boolean
}

export interface ProjectTabs {
  open: Tab[]
  active: string | null
}

interface TabsContextValue {
  tabsByProject: Record<string, ProjectTabs>
  saveError: string | null
  openFile: (projectId: string, path: string) => Promise<void>
  close: (projectId: string, path: string) => void
  setActive: (projectId: string, path: string) => void
  setDirty: (projectId: string, path: string, dirty: boolean) => void
  save: (projectId: string, path: string, content: string) => Promise<void>
  reload: (projectId: string, path: string, content: string) => void
  keepMine: (projectId: string, path: string) => void
}

const TabsContext = createContext<TabsContextValue | null>(null)

const titleOf = (path: string) => path.slice(path.lastIndexOf('/') + 1) || path

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
    async (projectId: string, path: string) => {
      setTabsByProject((prev) => {
        const st = prev[projectId] ?? { open: [], active: null }
        if (st.open.some((t) => t.path === path)) {
          return { ...prev, [projectId]: { ...st, active: path } }
        }
        const tab: Tab = { path, title: titleOf(path), dirty: false }
        return {
          ...prev,
          [projectId]: { open: [...st.open, tab], active: path },
        }
      })
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
      if (!tab) return
      if (op === 'remove' || op === 'rename') {
        if (!tab.dirty) close(projectId, path)
        return
      }
      const savedAt = lastSaveRef.current[`${projectId}\0${path}`]
      if (savedAt && Date.now() - savedAt < saveEchoWindowMs) return
      if (tab.dirty) {
        mutateTab(projectId, path, () => ({ staleExternally: true }))
        return
      }
      App.ReadFile(path)
        .then((content) => reload(projectId, path, content))
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
        setActive,
        setDirty,
        save,
        reload,
        keepMine,
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

import React, {
  createContext,
  useCallback,
  useContext,
  useState,
} from 'react'
import * as App from '../../bindings/aide/backend/app'

export interface Tab {
  path: string
  title: string
  dirContent?: string
  dirty: boolean
}

export interface ProjectTabs {
  open: Tab[]
  active: string | null
}

interface TabsContextValue {
  tabsByProject: Record<string, ProjectTabs>
  openFile: (projectId: string, path: string) => Promise<void>
  close: (projectId: string, path: string) => void
  setActive: (projectId: string, path: string) => void
  setDirty: (projectId: string, path: string, dirty: boolean) => void
}

const TabsContext = createContext<TabsContextValue | null>(null)

const titleOf = (path: string) => path.slice(path.lastIndexOf('/') + 1) || path

export const TabsProvider: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  const [tabsByProject, setTabsByProject] = useState<
    Record<string, ProjectTabs>
  >({})

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

  return (
    <TabsContext.Provider
      value={{ tabsByProject, openFile, close, setActive, setDirty }}
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

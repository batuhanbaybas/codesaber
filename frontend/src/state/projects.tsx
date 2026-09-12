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
import type { Project } from '../../bindings/aide/backend/project/models'

interface ProjectsContextValue {
  projects: Project[]
  activeId: string | null
  open: () => Promise<void>
  remove: (id: string) => Promise<void>
  openRecent: (root: string) => Promise<void>
  setActive: (id: string) => void
}

const ProjectsContext = createContext<ProjectsContextValue | null>(null)

const sortProjects = (ps: Project[]) =>
  [...ps].sort((a, b) => (a.lastUsed < b.lastUsed ? 1 : -1))

const upsert = (prev: Project[], p: Project) =>
  sortProjects([...prev.filter((x) => x.id !== p.id), p])

export const ProjectsProvider: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  const [projects, setProjects] = useState<Project[]>([])
  const [activeId, setActiveId] = useState<string | null>(null)
  const projectsRef = useRef(projects)
  projectsRef.current = projects
  const openInFlight = useRef(false)

  useEffect(() => {
    let disposed = false
    App.ListProjects().then((ps) => {
      if (disposed) return
      const list = ps ?? []
      setProjects(sortProjects(list))
      setActiveId((cur) => cur ?? list[0]?.id ?? null)
    })
    return () => {
      disposed = true
    }
  }, [])

  useEffect(() => {
    const onAdded = Events.On('project.added', (ev: any) => {
      const p = ev.data as Project
      if (!p?.id) return
      setProjects((prev) => upsert(prev, p))
      setActiveId((cur) => cur ?? p.id)
    })
    const onRemoved = Events.On('project.removed', (ev: any) => {
      const { id } = (ev.data ?? {}) as { id?: string }
      if (!id) return
      setProjects((prev) => prev.filter((x) => x.id !== id))
      setActiveId((cur) => {
        if (cur !== id) return cur
        const rest = projectsRef.current.filter((x) => x.id !== id)
        return rest[0]?.id ?? null
      })
    })
    return () => {
      onAdded()
      onRemoved()
    }
  }, [])

  const open = useCallback(async () => {
    if (openInFlight.current) return
    openInFlight.current = true
    try {
      const root = await App.PickFolder()
      if (root) {
        const p = await App.OpenProject(root)
        setProjects((prev) => upsert(prev, p))
        setActiveId(p.id)
      }
    } finally {
      openInFlight.current = false
    }
  }, [])

  const remove = useCallback(async (id: string) => {
    await App.RemoveProject(id)
  }, [])

  const openRecent = useCallback(async (root: string) => {
    const p = await App.OpenProject(root)
    setProjects((prev) => upsert(prev, p))
    setActiveId(p.id)
  }, [])

  const setActive = useCallback((id: string) => setActiveId(id), [])

  return (
    <ProjectsContext.Provider
      value={{ projects, activeId, open, remove, openRecent, setActive }}
    >
      {children}
    </ProjectsContext.Provider>
  )
}

export const useProjects = (): ProjectsContextValue => {
  const ctx = useContext(ProjectsContext)
  if (!ctx) throw new Error('useProjects must be used within ProjectsProvider')
  return ctx
}

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
import type { DiffPatch, Status } from '../../bindings/aide/backend/git/models'
import { useProjects } from './projects'

export const diffKey = (projectId: string, path: string, staged: boolean) =>
  `${projectId}:${path}:${staged ? 'staged' : 'unstaged'}`

interface GitContextValue {
  status: Record<string, Status>
  errors: Record<string, string>
  diffs: Record<string, DiffPatch>
  fetchDiff: (key: string) => void
  stage: (projectId: string, paths: string[]) => Promise<void>
  unstage: (projectId: string, paths: string[]) => Promise<void>
  commit: (projectId: string, message: string) => Promise<void>
}

const GitContext = createContext<GitContextValue | null>(null)

// parseDiffKey splits the synthetic key back into its parts. Paths may own
// colons, so only the first and last separators are treated as delimiters.
export const parseDiffKey = (
  key: string,
): { projectId: string; path: string; staged: boolean } | null => {
  const first = key.indexOf(':')
  const last = key.lastIndexOf(':')
  if (first === -1 || last <= first) return null
  const staged = key.slice(last + 1)
  if (staged !== 'staged' && staged !== 'unstaged') return null
  return {
    projectId: key.slice(0, first),
    path: key.slice(first + 1, last),
    staged: staged === 'staged',
  }
}

export const GitProvider: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  const [status, setStatus] = useState<Record<string, Status>>({})
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [diffs, setDiffs] = useState<Record<string, DiffPatch>>({})
  const diffsRef = useRef(diffs)
  diffsRef.current = diffs
  const { activeId, projects } = useProjects()

  const refresh = useCallback(async (projectId: string) => {
    if (!projectId) return
    try {
      const st = await App.GitStatus(projectId)
      setErrors((prev) => ({ ...prev, [projectId]: '' }))
      setStatus((prev) => ({ ...prev, [projectId]: st }))
    } catch {
      // not a repo / unmapped project — leave any error event to report it
    }
  }, [])

  useEffect(() => {
    const onStatus = Events.On('git.status', (ev: any) => {
      const { projectId, status: st } = (ev.data ?? {}) as {
        projectId?: string
        status?: Status
      }
      if (!projectId || !st) return
      setErrors((prev) => ({ ...prev, [projectId]: '' }))
      setStatus((prev) => ({ ...prev, [projectId]: st }))
    })
    const onError = Events.On('git.error', (ev: any) => {
      const { projectId, message } = (ev.data ?? {}) as {
        projectId?: string
        message?: string
      }
      if (!projectId || !message) return
      setErrors((prev) => ({ ...prev, [projectId]: message }))
    })
    return () => {
      onStatus()
      onError()
    }
  }, [])

  // Initial pull + any new project (activation or project.added) refreshes
  // the authoritative snapshot; throttled events keep it current afterwards.
  useEffect(() => {
    if (activeId) void refresh(activeId)
  }, [activeId, projects, refresh])

  const mutate = useCallback(
    async (
      projectId: string,
      fn: (paths: string[]) => Promise<void>,
      paths: string[],
    ) => {
      try {
        await fn(paths)
        setErrors((prev) => ({ ...prev, [projectId]: '' }))
      } catch (e) {
        setErrors((prev) => ({
          ...prev,
          [projectId]: String(e),
        }))
      }
      await refresh(projectId)
    },
    [refresh],
  )

  const stage = useCallback(
    (projectId: string, paths: string[]) =>
      mutate(projectId, (ps) => App.GitStage(projectId, ps), paths),
    [mutate],
  )

  const unstage = useCallback(
    (projectId: string, paths: string[]) =>
      mutate(projectId, (ps) => App.GitUnstage(projectId, ps), paths),
    [mutate],
  )

  const commit = useCallback(
    async (projectId: string, message: string) => {
      try {
        await App.GitCommit(projectId, message)
        setErrors((prev) => ({ ...prev, [projectId]: '' }))
      } catch (e) {
        setErrors((prev) => ({ ...prev, [projectId]: String(e) }))
      }
      await refresh(projectId)
    },
    [refresh],
  )

  const fetchDiff = useCallback((key: string) => {
    if (diffsRef.current[key]) return
    const parts = parseDiffKey(key)
    if (!parts) return
    App.GitDiff(parts.projectId, parts.path, parts.staged)
      .then((patch) =>
        setDiffs((prev) => ({ ...prev, [key]: patch })),
      )
      .catch(() => {})
  }, [])

  return (
    <GitContext.Provider
      value={{ status, errors, diffs, fetchDiff, stage, unstage, commit }}
    >
      {children}
    </GitContext.Provider>
  )
}

export const useGit = (): GitContextValue => {
  const ctx = useContext(GitContext)
  if (!ctx) throw new Error('useGit must be used within GitProvider')
  return ctx
}

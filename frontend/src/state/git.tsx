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
  /** Bumped whenever diff-cache entries are invalidated. */
  diffVersion: number
  /** Set/clear the per-project error shown as an inline red strip. */
  setError: (projectId: string, message: string) => void
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
  const [diffVersion, setDiffVersion] = useState(0)
  const diffsRef = useRef(diffs)
  diffsRef.current = diffs
  const { activeId, projects } = useProjects()

  const setError = useCallback((projectId: string, message: string) => {
    if (!projectId) return
    setErrors((prev) =>
      message === '' ? { ...prev, [projectId]: '' } : { ...prev, [projectId]: message },
    )
  }, [])

  // invalidateDiffs drops matching diff-cache entries and bumps the version
  // marker so open diff tabs refetch via their existing effect.
  const invalidateDiffs = useCallback(
    (projectId: string, paths?: string[]) => {
      const prev = diffsRef.current
      const pathSet = paths ? new Set(paths) : null
      const next: Record<string, DiffPatch> = {}
      let dropped = false
      for (const [key, patch] of Object.entries(prev)) {
        const parts = parseDiffKey(key)
        const match =
          parts !== null &&
          parts.projectId === projectId &&
          (pathSet === null || pathSet.has(parts.path))
        if (match) {
          dropped = true
          continue
        }
        next[key] = patch
      }
      if (dropped) {
        setDiffs(next)
        setDiffVersion((v) => v + 1)
      }
    },
    [],
  )

  const refresh = useCallback(
    async (projectId: string) => {
      if (!projectId) return
      try {
        const st = await App.GitStatus(projectId)
        setErrors((prev) => ({ ...prev, [projectId]: '' }))
        setStatus((prev) => ({ ...prev, [projectId]: st }))
      } catch {
        // not a repo / unmapped project — leave any error event to report it
      }
    },
    [],
  )

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
    // fs.change invalidates diff caches for the touched path so open diff
    // tabs refetch the fresh patch (if the path matches a cached entry).
    const onFsChange = Events.On('fs.change', (ev: any) => {
      const { projectId, path } = (ev.data ?? {}) as {
        projectId?: string
        path?: string
      }
      if (!projectId) return
      if (path) invalidateDiffs(projectId, [path])
    })
    const onRemoved = Events.On('project.removed', (ev: any) => {
      const { id } = (ev.data ?? {}) as { id?: string }
      if (!id) return
      invalidateDiffs(id)
      setErrors((prev) => {
        const next = { ...prev }
        delete next[id]
        return next
      })
      setStatus((prev) => {
        const next = { ...prev }
        delete next[id]
        return next
      })
    })
    return () => {
      onStatus()
      onError()
      onFsChange()
      onRemoved()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [invalidateDiffs])

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
      // completion (success or failure) invalidates the project's diffs
      invalidateDiffs(projectId)
      await refresh(projectId)
    },
    [refresh, invalidateDiffs],
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
        throw e // surface the failure to the caller; don't swallow it
      }
      invalidateDiffs(projectId)
      await refresh(projectId)
    },
    [refresh, invalidateDiffs],
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
      value={{
        status,
        errors,
        diffs,
        diffVersion,
        setError,
        fetchDiff,
        stage,
        unstage,
        commit,
      }}
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

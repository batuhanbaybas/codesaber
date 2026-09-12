import React, {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react'
import { Events } from '@wailsio/runtime'
import type { LSPDiagEvent, Diagnostic } from '../lsp'

export interface DiagCounts {
  errors: number
  warnings: number
}

const EMPTY: DiagCounts = { errors: 0, warnings: 0 }

// byProject: projectId → path → latest diagnostic list. A fresh "lsp.diag"
// event replaces the full list for its path (publishers send the complete
// set per publish), so counting is just a sum over stored lists.
const DiagContext = createContext<Record<string, Record<string, Diagnostic[]>>>({})

export const DiagCounterProvider: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  const [byProject, setByProject] = useState<
    Record<string, Record<string, Diagnostic[]>>
  >({})

  useEffect(() => {
    const off = Events.On('lsp.diag', (ev: any) => {
      const d = (ev.data ?? {}) as LSPDiagEvent
      if (!d?.projectId || !d.path) return
      const diags = d.diagnostics ?? []
      setByProject((prev) => {
        const paths = prev[d.projectId] ?? {}
        if ((paths[d.path] ?? []).length === 0 && diags.length === 0) return prev
        return { ...prev, [d.projectId]: { ...paths, [d.path]: diags } }
      })
    })
    return () => off()
  }, [])

  return (
    <DiagContext.Provider value={byProject}>{children}</DiagContext.Provider>
  )
}

// useDiagCounts aggregates error/warning counts for a project from the
// diagnostics streamed via "lsp.diag" (severity 1 = error, 2 = warning).
export const useDiagCounts = (projectId: string | null): DiagCounts => {
  const byProject = useContext(DiagContext)
  return useMemo(() => {
    if (!projectId) return EMPTY
    const paths = byProject[projectId]
    if (!paths) return EMPTY
    let errors = 0
    let warnings = 0
    for (const list of Object.values(paths)) {
      for (const d of list) {
        if (d.severity === 1) errors++
        else if (d.severity === 2) warnings++
      }
    }
    return { errors, warnings }
  }, [byProject, projectId])
}

import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
} from 'react'

// Window role is encoded in the URL hash by the backend window config
// (e.g. "#workspace"); fall back to the workspace role.
const role = window.location.hash.replace(/^#/, '') || 'workspace'

export interface LayoutUI {
  sidebar: boolean
  rightDock: boolean
  terminal: boolean
}

const defaults: LayoutUI = { sidebar: true, rightDock: true, terminal: true }

const storageKey = `aide.layout:${role}`

const load = (): LayoutUI => {
  try {
    const raw = window.localStorage.getItem(storageKey)
    if (!raw) return defaults
    const parsed = JSON.parse(raw) as Partial<LayoutUI>
    return { ...defaults, ...parsed }
  } catch {
    return defaults
  }
}

interface LayoutContextValue {
  ui: LayoutUI
  toggle: (key: keyof LayoutUI) => void
}

const LayoutContext = createContext<LayoutContextValue | null>(null)

export const LayoutProvider: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  const [ui, setUI] = useState<LayoutUI>(load)

  useEffect(() => {
    try {
      window.localStorage.setItem(storageKey, JSON.stringify(ui))
    } catch {
      // storage unavailable (private mode etc.) — layout just won't persist
    }
  }, [ui])

  const toggle = useCallback((key: keyof LayoutUI) => {
    setUI((prev) => ({ ...prev, [key]: !prev[key] }))
  }, [])

  return (
    <LayoutContext.Provider value={{ ui, toggle }}>
      {children}
    </LayoutContext.Provider>
  )
}

export const useLayout = (): LayoutContextValue => {
  const ctx = useContext(LayoutContext)
  if (!ctx) throw new Error('useLayout must be used within LayoutProvider')
  return ctx
}

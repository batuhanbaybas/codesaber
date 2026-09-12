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
  sidebarWidth: number
  rightDockWidth: number
  terminalHeight: number
}

export type SizeKey = 'sidebarWidth' | 'rightDockWidth' | 'terminalHeight'

export const SIZE_LIMITS: Record<SizeKey, { min: number; max: number; def: number }> = {
  sidebarWidth: { min: 180, max: 500, def: 240 },
  rightDockWidth: { min: 260, max: 700, def: 360 },
  terminalHeight: { min: 120, max: 600, def: 200 },
}

const defaults: LayoutUI = {
  sidebar: true,
  rightDock: true,
  terminal: true,
  sidebarWidth: SIZE_LIMITS.sidebarWidth.def,
  rightDockWidth: SIZE_LIMITS.rightDockWidth.def,
  terminalHeight: SIZE_LIMITS.terminalHeight.def,
}

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
  toggle: (key: 'sidebar' | 'rightDock' | 'terminal') => void
  setSize: (key: SizeKey, px: number) => void
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

  const toggle = useCallback((key: 'sidebar' | 'rightDock' | 'terminal') => {
    setUI((prev) => ({ ...prev, [key]: !prev[key] }))
  }, [])

  const setSize = useCallback((key: SizeKey, px: number) => {
    const { min, max } = SIZE_LIMITS[key]
    // Keep panels usable when the window is small: never let a panel eat
    // more than half the viewport along its axis.
    const viewportCap =
      key === 'terminalHeight' ? window.innerHeight / 2 : window.innerWidth / 2
    const clamped = Math.max(min, Math.min(max, viewportCap, px))
    setUI((prev) => (prev[key] === clamped ? prev : { ...prev, [key]: clamped }))
  }, [])

  return (
    <LayoutContext.Provider value={{ ui, toggle, setSize }}>
      {children}
    </LayoutContext.Provider>
  )
}

export const useLayout = (): LayoutContextValue => {
  const ctx = useContext(LayoutContext)
  if (!ctx) throw new Error('useLayout must be used within LayoutProvider')
  return ctx
}

// Tiny persisted-settings store for toggleable editor preferences.
// Values live in localStorage; changes broadcast a window event so
// useSyncExternalStore consumers update reactively.

const KEY = 'aide.bracketColors'
const HUD_KEY = 'aide.perfHud'
export const SETTINGS_EVENT = 'aide:settings'

export const bracketColorsEnabled = (): boolean =>
  window.localStorage.getItem(KEY) !== 'false'

export const setBracketColors = (v: boolean): void => {
  try {
    window.localStorage.setItem(KEY, String(v))
  } catch {
    // storage unavailable — setting just won't persist
  }
  window.dispatchEvent(new Event(SETTINGS_EVENT))
}

export const toggleBracketColors = (): boolean => {
  const next = !bracketColorsEnabled()
  setBracketColors(next)
  return next
}

export const onSettingsChange = (fn: () => void): (() => void) => {
  window.addEventListener(SETTINGS_EVENT, fn)
  return () => window.removeEventListener(SETTINGS_EVENT, fn)
}

export const perfHudEnabled = (): boolean =>
  window.localStorage.getItem(HUD_KEY) === 'true'

export const setPerfHud = (v: boolean): void => {
  try {
    window.localStorage.setItem(HUD_KEY, String(v))
  } catch {
    // storage unavailable — setting just won't persist
  }
  window.dispatchEvent(new Event(SETTINGS_EVENT))
}

export const togglePerfHud = (): boolean => {
  const next = !perfHudEnabled()
  setPerfHud(next)
  return next
}

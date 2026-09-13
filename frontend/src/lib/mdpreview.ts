// Per-tab markdown preview mode. Kept out of the TabsProvider so closing a
// preview at the last second does not round-trip tab state; a module store
// keyed by tab path is enough.

const listeners = new Set<() => void>()
const enabled = new Map<string, boolean>()

export const mdPreviewOn = (path: string): boolean =>
  enabled.get(path) ?? false

export const setMdPreview = (path: string, on: boolean): void => {
  if (on === (enabled.get(path) ?? false)) return
  if (on) enabled.set(path, true)
  else enabled.delete(path)
  listeners.forEach((l) => l())
}

export const toggleMdPreview = (path: string): void =>
  setMdPreview(path, !mdPreviewOn(path))

export const subscribeMdPreview = (l: () => void): (() => void) => {
  listeners.add(l)
  return () => listeners.delete(l)
}

export const isMarkdown = (path: string): boolean =>
  /\.(md|markdown)$/i.test(path)

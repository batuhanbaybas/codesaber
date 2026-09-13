import React, { useEffect, useRef } from 'react'

// ConfirmDialog is an in-app confirmation modal. window.confirm is not
// usable here: WKWebView in Wails v3 does not implement JS modal dialogs,
// so confirm() never shows and always aborts the action.

interface Props {
  title: string
  message: string
  confirmLabel?: string
  danger?: boolean
  onConfirm: () => void
  onCancel: () => void
}

const ConfirmDialog: React.FC<Props> = ({
  title,
  message,
  confirmLabel = 'Confirm',
  danger,
  onConfirm,
  onCancel,
}) => {
  const btnRef = useRef<HTMLButtonElement | null>(null)
  useEffect(() => {
    btnRef.current?.focus()
  }, [])
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        e.stopPropagation()
        onCancel()
      } else if (e.key === 'Enter') {
        e.preventDefault()
        e.stopPropagation()
        onConfirm()
      }
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [onCancel, onConfirm])
  return (
    <div
      className="fixed inset-0 z-[60] flex items-center justify-center bg-black/40"
      onMouseDown={onCancel}
    >
      <div
        className="w-[320px] rounded-lg border border-[var(--bg-border)] bg-[var(--bg-panel)] shadow-xl p-4 text-xs"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="text-primary text-[13px] font-medium mb-1">{title}</div>
        <div className="text-dim mb-4">{message}</div>
        <div className="flex justify-end gap-2">
          <button
            className="no-drag px-3 py-1.5 rounded border border-[var(--bg-border)] text-dim hover:text-primary hover:bg-white/8"
            onClick={onCancel}
          >
            Cancel
          </button>
          <button
            ref={btnRef}
            className={
              'no-drag px-3 py-1.5 rounded text-white ' +
              (danger
                ? 'bg-[#c75454] hover:bg-[#d86666]'
                : 'bg-[#2b4d75] hover:bg-[#35597e]')
            }
            onClick={onConfirm}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}

export default ConfirmDialog

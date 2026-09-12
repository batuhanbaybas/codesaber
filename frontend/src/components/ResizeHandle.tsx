import React, { useCallback, useRef } from 'react'

interface ResizeHandleProps {
  axis: 'x' | 'y'
  onResize: (sizePx: number) => void
  onReset: () => void
  getCurrent: () => number
  minWidth?: number
  /** Negate the pointer delta (e.g. resizing a panel by its left edge). */
  flip?: boolean
}

// 4px gutter between panels. Pointer capture tracks the drag as a delta from
// the panel's start size; clamping/persistence are the parent's job.
const ResizeHandle: React.FC<ResizeHandleProps> = ({
  axis,
  onResize,
  onReset,
  getCurrent,
  minWidth = 4,
  flip = false,
}) => {
  const startYRef = useRef(0)
  const startSizeRef = useRef(0)

  const onPointerDown = useCallback(
    (e: React.PointerEvent<HTMLDivElement>) => {
      if (e.button !== 0) return
      const el = e.currentTarget
      el.setPointerCapture(e.pointerId)
      startYRef.current = axis === 'y' ? e.clientY : e.clientX
      startSizeRef.current = getCurrent()
      document.body.dataset.resizing = '1'
    },
    [axis, getCurrent],
  )

  const onPointerMove = useCallback(
    (e: React.PointerEvent<HTMLDivElement>) => {
      if (document.body.dataset.resizing !== '1') return
      const delta =
        axis === 'y' ? e.clientY - startYRef.current : e.clientX - startYRef.current
      onResize(Math.max(minWidth, startSizeRef.current + (flip ? -delta : delta)))
    },
    [axis, onResize, minWidth],
  )

  const endDrag = useCallback(
    (e: React.PointerEvent<HTMLDivElement>) => {
      if (document.body.dataset.resizing !== '1') return
      delete document.body.dataset.resizing
      try {
        e.currentTarget.releasePointerCapture(e.pointerId)
      } catch {
        /* pointer already released */
      }
      onResize(getCurrent())
    },
    [onResize, getCurrent],
  )

  const onKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      if (e.key === 'Enter') onReset()
    },
    [onReset],
  )

  return (
    <div
      className={
        'no-drag shrink-0 z-10 group ' +
        (axis === 'y' ? 'row-resize h-1 w-full cursor-row-resize' : 'col-resize w-1 h-full cursor-col-resize')
      }
      role="separator"
      tabIndex={0}
      aria-orientation={axis === 'y' ? 'horizontal' : 'vertical'}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={endDrag}
      onPointerCancel={endDrag}
      onDoubleClick={onReset}
      onKeyDown={onKeyDown}
    />
  )
}

export default ResizeHandle

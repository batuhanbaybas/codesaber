import React, { useEffect, useMemo, useState } from 'react'

// MiniDiff is the lightweight preview renderer for agent fs-write permission
// cards. It computes a line-level diff (LCS, capped) between the on-disk text
// and the requested text and renders it either side-by-side (container
// >= 480px) or as a compact two-column +/- list. No CodeMirror, no external
// deps — deliberately minimal next to the full DiffViewer.

interface Row {
  op: '+' | '-' | ' '
  text: string
  // side-by-side pairing: rows beyond this width render as blank filler
  peer?: string
  peerOp?: '+' | '-' | ' '
}

const MAX_DIFF_LINES = 400

// diffRows computes line rows via capped LCS. Files larger than the cap
// degrade to a head-only preview with an ellipsis marker row.
const diffRows = (oldText: string, newText: string): Row[] => {
  const oldArr = oldText.split('\n')
  const newArr = newText.split('\n')
  // normalize: trailing newline produces a final '' — drop it for pairing
  if (oldArr.length > 1 && oldArr[oldArr.length - 1] === '') oldArr.pop()
  if (newArr.length > 1 && newArr[newArr.length - 1] === '') newArr.pop()
  const truncated =
    oldArr.length > MAX_DIFF_LINES || newArr.length > MAX_DIFF_LINES
  const a = oldArr.slice(0, MAX_DIFF_LINES)
  const b = newArr.slice(0, MAX_DIFF_LINES)

  // LCS DP (capped at 400x400 => 160k cells, fine)
  const m = a.length
  const n = b.length
  const dp: Uint32Array[] = []
  for (let i = 0; i <= m; i++) dp.push(new Uint32Array(n + 1))
  for (let i = m - 1; i >= 0; i--)
    for (let j = n - 1; j >= 0; j--)
      dp[i][j] =
        a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1])

  const rows: Row[] = []
  let i = 0
  let j = 0
  while (i < m && j < n) {
    if (a[i] === b[j]) {
      rows.push({ op: ' ', text: a[i] })
      i++
      j++
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      rows.push({ op: '-', text: a[i] })
      i++
    } else {
      rows.push({ op: '+', text: b[j] })
      j++
    }
  }
  for (; i < m; i++) rows.push({ op: '-', text: a[i] })
  for (; j < n; j++) rows.push({ op: '+', text: b[j] })
  if (truncated) rows.push({ op: ' ', text: '\u2026' })
  return rows
}

// pairRows folds +/- runs so side-by-side can show old|new columns.
const pairRows = (rows: Row[]): Row[] => {
  const out: Row[] = []
  let k = 0
  const isPlus = (r: Row | undefined) => r !== undefined && r.op === '+'
  const isMinus = (r: Row | undefined) => r !== undefined && r.op === '-'
  while (k < rows.length) {
    const r = rows[k]
    if (isMinus(r)) {
      // collect a delete block, then pair with an equal-length insert block
      let dels = 0
      let adds = 0
      while (k + dels < rows.length && isMinus(rows[k + dels])) dels++
      while (
        k + dels + adds < rows.length &&
        isPlus(rows[k + dels + adds]) &&
        adds < dels
      )
        adds++
      if (adds > 0) {
        for (let x = 0; x < dels; x++) {
          out.push({
            op: '-',
            text: rows[k + x].text,
            peer: x < adds ? rows[k + dels + x].text : undefined,
            peerOp: x < adds ? '+' : undefined,
          })
        }
        // leftover insert rows beyond deletion pairing
        for (let x = dels; x < adds; x++) {
          out.push({ op: '+', text: rows[k + dels + x].text })
        }
        k += dels + adds
      } else {
        out.push(r)
        k++
      }
    } else if (isPlus(r)) {
      out.push(r)
      k++
    } else {
      out.push(r)
      k++
    }
  }
  return out
}

// useWidth tracks the rendered card width to pick the layout mode.
const useWidth = (): [React.RefObject<HTMLDivElement>, number] => {
  const [w, setW] = useState(0)
  const ref = React.useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = ref.current
    if (!el || typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver((entries) => {
      for (const e of entries) setW(e.contentRect.width)
    })
    observer.observe(el)
    return () => observer.disconnect()
  }, [])
  return [ref, w]
}

// diffCounts returns the +/- line counts of the (capped) diff — used by card
// headers; MiniDiff itself counts via the same helper.
export const diffCounts = (oldText: string, newText: string) => {
  const rows = diffRows(oldText, newText)
  return counts(rows)
}

const addedBg = 'rgba(13,140,50,0.13)'
const delBg = 'rgba(200,50,50,0.13)'

const counts = (rows: Row[]): { adds: number; dels: number } => {
  let adds = 0
  let dels = 0
  for (const r of rows) {
    if (r.op === '+') adds++
    else if (r.op === '-') dels++
    if (r.peerOp === '+') adds++
  }
  return { adds, dels }
}

export const MiniDiff: React.FC<{
  oldText: string
  newText: string
  maxLines?: number
}> = ({ oldText, newText, maxLines = 200 }) => {
  const [ref, width] = useWidth()
  const rows = useMemo(() => diffRows(oldText, newText), [oldText, newText])
  const wide = width >= 480
  const shown = rows.slice(0, maxLines)
  const side = useMemo(() => (wide ? pairRows(shown) : shown), [wide, shown])

  return (
    <div ref={ref} className="min-w-0">
      {!wide ? (
        <pre className="mt-1 rounded bg-[#141517] px-2 py-1 font-mono text-[10px] leading-4 max-h-40 overflow-y-auto overflow-x-hidden">
          {side.map((r, i) => (
            <div
              key={i}
              className={
                'whitespace-pre-wrap break-all ' +
                (r.op === '+' ? 'text-[#8ee0a0]' : r.op === '-' ? 'text-[#ef8070]' : 'text-dim')
              }
              style={{
                backgroundColor:
                  r.op === '+' ? addedBg : r.op === '-' ? delBg : undefined,
              }}
            >
              {r.op === ' ' ? '  ' : r.op + ' '}
              {r.text === '' ? ' ' : r.text}
            </div>
          ))}
        </pre>
      ) : (
        <pre className="mt-1 rounded bg-[#141517] px-2 py-1 font-mono text-[10px] leading-4 max-h-40 overflow-y-auto overflow-x-hidden">
          {side.map((r, i) => (
            <div key={i} className="flex">
              <div
                className="w-1/2 pr-1 whitespace-pre-wrap break-all"
                style={{ backgroundColor: r.op === '-' ? delBg : undefined }}
              >
                <span
                  className={
                    'select-none ' +
                    (r.op === '-' ? 'text-[#ef8070]' : 'text-dim')
                  }
                >
                  {r.op === '-' ? '−' : ''}
                </span>
                <span className={r.op === '-' ? 'text-[#ef8070]' : 'text-dim'}>
                  {r.text}
                </span>
              </div>
              <div
                className="w-1/2 pl-1 whitespace-pre-wrap break-all"
                style={{ backgroundColor: r.peerOp === '+' ? addedBg : undefined }}
              >
                <span
                  className={
                    'select-none ' +
                    (r.peerOp === '+' ? 'text-[#8ee0a0]' : 'text-dim')
                  }
                >
                  {r.peerOp === '+' ? '+' : ''}
                </span>
                <span className={r.peerOp === '+' ? 'text-[#8ee0a0]' : 'text-dim'}>
                  {r.peer ?? ''}
                </span>
              </div>
            </div>
          ))}
        </pre>
      )}
      {rows.length > maxLines && (
        <div className="mt-0.5 text-[10px] text-dim">
          (showing first {maxLines} rows)
        </div>
      )}
    </div>
  )
}

export default MiniDiff

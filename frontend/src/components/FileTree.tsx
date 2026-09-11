import React from 'react'

interface Node {
  name: string
  depth: number
  dir?: boolean
  caret?: string
}

const demo: Node[] = [
  { name: 'aide', depth: 0, dir: true, caret: '\u25be' },
  { name: 'backend', depth: 1, dir: true, caret: '\u25be' },
  { name: 'app.go', depth: 2 },
  { name: 'main.go', depth: 2 },
  { name: 'frontend', depth: 1, dir: true, caret: '\u25be' },
  { name: 'index.html', depth: 2 },
  { name: 'package.json', depth: 2 },
  { name: 'go.mod', depth: 0 },
]

const FileTree: React.FC = () => {
  return (
    <div className="py-1">
      {demo.map((node, i) => (
        <div
          key={i}
          className="flex items-center gap-1 py-[3px] pr-2 hover:bg-[#373940] cursor-default"
          style={{ paddingLeft: 8 + node.depth * 14 }}
        >
          <span className="w-2 text-dim text-[9px]">{node.caret ?? ''}</span>
          <span className={node.dir ? 'text-primary' : 'text-dim'}>{node.name}</span>
        </div>
      ))}
    </div>
  )
}

export default FileTree

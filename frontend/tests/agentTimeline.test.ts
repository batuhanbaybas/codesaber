import { describe, it, expect } from 'vitest'
import {
  emptyAgentState,
  reduceMsg,
  reduceTool,
  reducePermission,
  reduceStateFlip,
  reduceTranscript,
  type TimelineItem,
  type ToolItem,
  type PermissionItem,
} from '../src/state/agentTimeline'

describe('agent timeline reducer', () => {
  it('creates one message per chunk-batch and merges the tail', () => {
    let s = emptyAgentState()
    s = reduceMsg(s, { role: 'agent', text: 'Hel', kind: 'chunk' })
    s = reduceMsg(s, { role: 'agent', text: 'lo', kind: 'chunk' })
    s = reduceMsg(s, { role: 'user', text: 'hi', kind: 'text' })
    s = reduceMsg(s, { role: 'agent', text: 'ag', kind: 'chunk' })
    const tl = s.timeline
    expect(tl).toHaveLength(3)
    expect((tl[0] as any).text).toBe('Hello')
    expect(tl[1].type).toBe('message')
    expect((tl[2] as any).text).toBe('ag')
  })

  it('upserts tool cards in place by toolCallId', () => {
    let s = emptyAgentState()
    s = reduceTool(s, { toolCallId: 't1', title: 'Read x', status: 'in_progress', kind: 'read', content: '' })
    s = reduceTool(s, { toolCallId: 't2', title: 'Grep y', status: 'in_progress', kind: 'search', content: '' })
    s = reduceTool(s, { toolCallId: 't1', title: 'Read x', status: 'completed', kind: 'read', content: 'out' })
    const tools = s.timeline.filter((i) => i.type === 'tool') as ToolItem[]
    expect(tools).toHaveLength(2)
    expect(tools[0].status).toBe('completed')
    expect(tools[0].content).toBe('out')
    expect(s.timeline.map((i) => i.type)).toEqual(['tool', 'tool'])
  })

  it('stacks permissions and drops them on state flip to thinking', () => {
    let s = emptyAgentState()
    s = reducePermission(s, { requestId: 'p1', options: [] })
    s = reducePermission(s, { requestId: 'p2', options: [] })
    s = reducePermission(s, { requestId: 'p1', options: [] }) // dedupe
    expect(s.timeline.filter((i) => i.type === 'permission')).toHaveLength(2)
    s = reduceStateFlip(s, 'thinking')
    expect(s.timeline.filter((i) => i.type === 'permission')).toHaveLength(0)
  })

  it('rebuilds the timeline from transcript entries interleaved', () => {
    const entries = [
      { role: 'user', text: 'go', kind: 'text', when: '2026-01-01T00:00:00Z' },
      { role: 'agent', text: 'Reading', kind: 'tool', toolId: 't1', when: '2026-01-01T00:00:01Z' },
      { role: 'agent', text: 'done', kind: 'text', when: '2026-01-01T00:00:02Z' },
    ]
    const s = reduceTranscript(emptyAgentState(), entries)
    expect(s.timeline.map((i) => i.type)).toEqual(['message', 'tool', 'message'])
    expect((s.timeline[0] as any).role).toBe('user')
    expect((s.timeline[1] as any).toolCallId).toBe('t1')
    expect((s.timeline[2] as any).text).toBe('done')
  })
})

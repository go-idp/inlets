import { describe, it, expect } from 'vitest'
import { parseDiff, computeDiff } from './DiffViewer'

describe('parseDiff', () => {
  it('returns empty for empty input', () => {
    expect(parseDiff('')).toEqual([])
  })

  it('parses kept/added/removed lines', () => {
    const raw = ' a\n-b\n+c\n'
    const lines = parseDiff(raw)
    expect(lines).toEqual([
      { kind: ' ', text: 'a' },
      { kind: '-', text: 'b' },
      { kind: '+', text: 'c' },
    ])
  })
})

describe('computeDiff', () => {
  it('produces only kept lines for identical inputs', () => {
    const d = computeDiff('a\nb', 'a\nb')
    expect(d).toBe(' a\n b')
  })

  it('produces an addition marker for new lines', () => {
    const d = computeDiff('a\n', 'a\nb\n')
    expect(d.split('\n').some((l) => l.startsWith('+b'))).toBe(true)
  })

  it('produces a removal marker for deleted lines', () => {
    const d = computeDiff('a\nb\n', 'a\n')
    expect(d.split('\n').some((l) => l.startsWith('-b'))).toBe(true)
  })
})

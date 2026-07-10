import { describe, expect, it } from 'vitest'

import { resolveStoryIndex, shouldReducePublicMotion, splitCharacters } from '../publicMotion'

describe('public motion helpers', () => {
  it('splits unicode text without breaking surrogate pairs', () => {
    expect(splitCharacters('API精灵球⚡')).toEqual(['A', 'P', 'I', '精', '灵', '球', '⚡'])
  })

  it('maps clamped scroll progress to story scenes', () => {
    expect(resolveStoryIndex(-1, 3)).toBe(0)
    expect(resolveStoryIndex(0.32, 3)).toBe(0)
    expect(resolveStoryIndex(0.34, 3)).toBe(1)
    expect(resolveStoryIndex(0.67, 3)).toBe(2)
    expect(resolveStoryIndex(2, 3)).toBe(2)
    expect(resolveStoryIndex(Number.NaN, 3)).toBe(0)
  })

  it('disables expensive motion for accessibility and constrained devices', () => {
    expect(shouldReducePublicMotion({ reducedMotion: true, coarsePointer: false })).toBe(true)
    expect(shouldReducePublicMotion({ reducedMotion: false, coarsePointer: true })).toBe(true)
    expect(
      shouldReducePublicMotion({ reducedMotion: false, coarsePointer: false, saveData: true })
    ).toBe(true)
    expect(shouldReducePublicMotion({ reducedMotion: false, coarsePointer: false })).toBe(false)
  })
})

import { describe, expect, it } from 'vitest'
import { shouldShrinkFieldLabel } from './fieldLabel'

describe('shouldShrinkFieldLabel', () => {
  it('shrinks text labels after asynchronous detail values arrive', () => {
    expect(shouldShrinkFieldLabel(undefined, 'admin')).toBe(true)
    expect(shouldShrinkFieldLabel('textarea', '備考')).toBe(true)
  })

  it('keeps empty text fields normal and always shrinks date labels', () => {
    expect(shouldShrinkFieldLabel(undefined, '')).toBe(false)
    expect(shouldShrinkFieldLabel('date', '')).toBe(true)
  })
})

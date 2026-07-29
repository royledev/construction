import { describe, expect, it, vi } from 'vitest'
import { editableCellA11y, isEditActivation } from './accessibility'
import { RevisionSaveQueue } from './autosaveQueue'

describe('RevisionSaveQueue', () => {
  it('restores a failed snapshot and merges the next edit before retrying', async () => {
    vi.useFakeTimers()
    const request = vi.fn().mockRejectedValueOnce(new Error('network')).mockResolvedValueOnce({ revision: 2 })
    const queue = new RevisionSaveQueue(1, 20, request, vi.fn(), vi.fn())
    queue.enqueue({ started_at: '09:00' })
    await queue.flush()
    expect(queue.hasPending()).toBe(true)
    queue.enqueue({ ended_at: '17:00' })
    await vi.runAllTimersAsync()
    expect(request).toHaveBeenNthCalledWith(2, { started_at: '09:00', ended_at: '17:00' }, 1)
    vi.useRealTimers()
  })

  it('flushes pending work during disposal', async () => {
    const request = vi.fn().mockResolvedValue({ revision: 2 })
    const queue = new RevisionSaveQueue(1, 1000, request, vi.fn(), vi.fn())
    queue.enqueue({ work_description: '配管' })
    queue.dispose(true)
    await Promise.resolve()
    expect(request).toHaveBeenCalledWith({ work_description: '配管' }, 1)
  })

  it('accepts one debounced save from a replacement queue after StrictMode cleanup', async () => {
    vi.useFakeTimers()
    const request = vi.fn().mockResolvedValue({ revision: 2 })
    // React StrictMode mounts, cleans up, then mounts effects again. The first
    // queue is disposed during that probe and must be replaced before editing.
    const probedQueue = new RevisionSaveQueue(1, 20, request, vi.fn(), vi.fn())
    probedQueue.dispose(true)
    expect(probedQueue.isDisposed()).toBe(true)

    const activeQueue = new RevisionSaveQueue(1, 20, request, vi.fn(), vi.fn())
    activeQueue.enqueue({ work_description: 'StrictMode保存' })
    await vi.advanceTimersByTimeAsync(20)

    expect(request).toHaveBeenCalledTimes(1)
    expect(request).toHaveBeenCalledWith({ work_description: 'StrictMode保存' }, 1)
    vi.useRealTimers()
  })

  it('drains B after disposal when A was already in flight', async () => {
    let resolveA: ((value: { revision: number }) => void) | undefined
    const request = vi.fn().mockImplementationOnce(() => new Promise<{ revision: number }>(resolve => { resolveA = resolve })).mockResolvedValueOnce({ revision: 3 })
    const queue = new RevisionSaveQueue(1, 0, request, vi.fn(), vi.fn())
    queue.enqueue({ started_at: '09:00' })
    const inFlight = queue.flush()
    queue.enqueue({ ended_at: '17:00' })
    queue.dispose(true)
    resolveA?.({ revision: 2 })
    await inFlight
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(request).toHaveBeenNthCalledWith(2, { ended_at: '17:00' }, 2)
  })

  it('re-enables after a 409 reload and uses the refreshed revision', async () => {
    const request = vi.fn().mockRejectedValueOnce(new Error('409 競合')).mockResolvedValueOnce({ revision: 3 })
    const queue = new RevisionSaveQueue(1, 0, request, vi.fn(), error => { if (String(error).includes('409')) queue.pause() })
    queue.enqueue({ work_description: '古い入力' })
    await queue.flush()
    // This models the same mounted component receiving a reloaded row.
    queue.setRevision(2)
    queue.enqueue({ work_description: '再読込後の入力' })
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(request).toHaveBeenNthCalledWith(2, { work_description: '再読込後の入力' }, 2)
  })
})

describe('editable cell accessibility', () => {
  it('exposes a keyboard-operable Japanese button label', () => {
    expect(editableCellA11y('作業内容')).toEqual({ role: 'button', tabIndex: 0, 'aria-label': '作業内容を編集' })
    expect(isEditActivation('Enter')).toBe(true)
    expect(isEditActivation(' ')).toBe(true)
    expect(isEditActivation('Escape')).toBe(false)
  })
})

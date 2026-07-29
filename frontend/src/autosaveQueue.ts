export type Revisioned = { revision: number }

// A cell/row owns one queue. A failed snapshot is merged back ahead of newer
// edits, so retrying never drops a value that was already sent to the server.
export class RevisionSaveQueue<T extends Revisioned> {
  private revision: number
  private pending: Record<string, unknown> = {}
  private timer: ReturnType<typeof setTimeout> | undefined
  private saving = false
  private disposed = false
  private draining = false
  private paused = false
  private generation = 0

  constructor(
    revision: number,
    private readonly delay: number,
    private readonly request: (changes: Record<string, unknown>, revision: number) => Promise<T>,
    private readonly success: (item: T) => void,
    private readonly failure: (error: unknown) => void,
  ) { this.revision = revision }

  setRevision(revision: number) {
    if (this.paused) {
      const editsMadeAfterPause = this.pending
      this.reset(revision)
      this.pending = editsMadeAfterPause
      if (this.hasPending()) this.schedule(this.delay)
      return
    }
    if (!this.saving && Object.keys(this.pending).length === 0) this.revision = revision
  }
  enqueue(changes: Record<string, unknown>) { this.pending = { ...this.pending, ...changes }; if (!this.paused) this.schedule(this.delay) }
  retry() { this.schedule(0) }
  hasPending() { return Object.keys(this.pending).length > 0 }
	// Hooks use this to replace a queue after React StrictMode's development
	// setup → cleanup → setup probe has disposed the previous instance.
	isDisposed() { return this.disposed }
  pause() { clearTimeout(this.timer); this.generation++; this.pending = {}; this.saving = false; this.paused = true; this.draining = false }
  reset(revision: number) { clearTimeout(this.timer); this.generation++; this.revision = revision; this.pending = {}; this.saving = false; this.paused = false; this.disposed = false; this.draining = false }
  dispose(flush = true) {
    clearTimeout(this.timer)
    this.disposed = true
    if (flush) { this.draining = true; if (!this.saving) void this.flush() } else { this.generation++; this.draining = false }
  }

  async flush() {
    clearTimeout(this.timer)
    if (this.saving || this.paused || (this.disposed && !this.draining) || !this.hasPending()) return
    const sent = this.pending
    this.pending = {}
    this.saving = true
    const generation = this.generation
    let failed = false
    try {
      const item = await this.request(sent, this.revision)
      if (generation !== this.generation) return
      this.revision = item.revision
      this.success(item)
    } catch (error) {
      if (generation !== this.generation) return
      // New local edits win when they target the same key; otherwise retain
      // both the failed request and edits made while it was in flight.
      this.pending = { ...sent, ...this.pending }
      failed = true
      this.failure(error)
    } finally {
      if (generation === this.generation) {
        this.saving = false
        if (!failed && (!this.disposed || this.draining) && this.hasPending()) this.schedule(0)
        if (this.draining && !this.hasPending() && !this.saving) this.draining = false
      }
    }
  }

  private schedule(delay: number) { clearTimeout(this.timer); if ((!this.disposed || this.draining) && !this.paused) this.timer = setTimeout(() => void this.flush(), delay) }
}

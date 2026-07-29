// Controlled fields receive asynchronous detail values after their first
// render. Explicit shrink keeps the label from overlapping that value.
export function shouldShrinkFieldLabel(type: string | undefined, value: unknown): boolean {
  return type === 'date' || String(value ?? '') !== ''
}

const statusLabels: Record<string, string> = {
  active: '有効・在籍', inactive: '無効', retired: '退職',
  unreceived: '未受注', ordered: '受注', completed: '完了', lost: '失注',
}

const roleLabels: Record<string, string> = { admin: '管理者', user: '一般' }
const workerTypeLabels: Record<string, string> = { employee: '自社', subcontractor: '外注先' }

// Values such as "admin" are valid login IDs. Translate only fields whose
// schema defines those codes, never arbitrary ledger cells.
export function displayLedgerValue(key: string, value: unknown): string {
  if (value === null || value === undefined || value === '') return '—'
  const text = String(value)
  if (key === 'status') return statusLabels[text] ?? text
  if (key === 'role') return roleLabels[text] ?? text
  if (key === 'worker_type') return workerTypeLabels[text] ?? text
  return text
}

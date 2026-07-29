import type { LedgerRecord, ScheduleSlot, WorkTrackingRecord } from './api'

export type DailyView = 'overview' | 'mine' | 'preview'
export type DailyCompanyGroup = { companyName: string; workers: DailyWorkerGroup[] }
export type DailyWorkerGroup = { key: string; name: string; occupation: string; rows: WorkTrackingRecord[] }
export type ScheduleWorker = LedgerRecord & { worker_type: 'employee' | 'subcontractor'; company_name: string }
export type ScheduleRow = { companyName: string; worker: ScheduleWorker; slot: number; companyRowSpan: number; firstInCompany: boolean }

export const asText = (value: unknown) => value === null || value === undefined ? '' : String(value)

// Components start a token for every fetch and only commit the response that
// still owns the newest token. This keeps a slow former month/view response
// from replacing the current screen.
export type RequestToken = { generation: number; key: string }
export class LatestRequest {
  private generation = 0
  begin(key: string): RequestToken { this.generation += 1; return { generation: this.generation, key } }
  accepts(token: RequestToken, currentKey: string) { return token.generation === this.generation && token.key === currentKey }
  invalidate() { this.generation += 1 }
}

export function dailyScopeKey(month: string, view: DailyView) { return `${month}:${view}` }
export function scopedItems<T>(state: { key: string; items: T[] }, key: string) { return state.key === key ? state.items : [] }
export function canCommitScope(startKey: string, currentKey: string) { return startKey === currentKey }

export function reportOrder(a: WorkTrackingRecord, b: WorkTrackingRecord) {
  const date = asText(a.report_date).localeCompare(asText(b.report_date))
  if (date) return date
  const groupA = Number(a.parent_report_id ?? a.id)
  const groupB = Number(b.parent_report_id ?? b.id)
  return groupA - groupB || Number(Boolean(a.parent_report_id)) - Number(Boolean(b.parent_report_id)) || a.id - b.id
}

// A reload can already contain the row returned by a concurrent POST. Treat
// the POST result as an upsert, never an append, so the grid stays unique.
export function upsertReport(rows: WorkTrackingRecord[], item: WorkTrackingRecord) {
  return [...rows.filter(row => row.id !== item.id), item].sort(reportOrder)
}

export type BusyOperation = { operationId: number; key: string }
export function releaseBusyOperation<T extends BusyOperation>(operations: T[], operationId: number) {
  return operations.filter(operation => operation.operationId !== operationId)
}

export function canDeleteDailyReport(rows: WorkTrackingRecord[], target: WorkTrackingRecord) {
  return rows.filter(row => asText(row.report_date) === asText(target.report_date)).length >= 2
}

export function nextDailyView(current: DailyView, action: 'browse' | 'input' | 'preview'): DailyView {
  if (action === 'browse') return 'overview'
  if (action === 'input') return 'mine'
  return 'preview'
}

export function viewForMonth(view: DailyView, previewMonth: string | undefined, month: string): DailyView {
  return view === 'preview' && previewMonth !== month ? 'overview' : view
}

export function canAddScheduleSlot(role: 'admin' | 'user') { return role === 'admin' }

export function groupDailyReports(rows: WorkTrackingRecord[]): DailyCompanyGroup[] {
  const companies = new Map<string, Map<string, DailyWorkerGroup>>()
  for (const row of rows) {
    const external = asText(row.worker_type) === 'subcontractor'
    const companyName = asText(row.company_name) || (external ? asText(row.subcontractor_name) : '自社')
    const workerName = external ? asText(row.subcontractor_worker_name) : asText(row.employee_name)
    const subjectID = external ? row.subcontractor_worker_id ?? row.worker_id : row.employee_id ?? row.worker_id
    // The display name is intentionally not part of identity: two workers can
    // share a name within one company.
    const key = `${external ? 'subcontractor' : 'employee'}:${asText(subjectID) || `report-${row.id}`}`
    const workers = companies.get(companyName) ?? new Map<string, DailyWorkerGroup>()
    companies.set(companyName, workers)
    const group = workers.get(key) ?? { key, name: workerName || '名称未設定', occupation: asText(row.occupation), rows: [] }
    group.rows.push(row)
    workers.set(key, group)
  }
  return [...companies.entries()].map(([companyName, workers]) => ({
    companyName,
    workers: [...workers.values()].map(worker => ({ ...worker, rows: [...worker.rows].sort(reportOrder) })),
  }))
}

export function dateLabels(month: string) {
  const days = new Date(Number(month.slice(0, 4)), Number(month.slice(5, 7)), 0).getDate()
  return Array.from({ length: days }, (_, index) => {
    const day = index + 1
    return { date: `${month}-${String(day).padStart(2, '0')}`, label: `${Number(month.slice(5, 7))}月${day}日` }
  })
}

export function scheduleRows(companyName: string, employees: LedgerRecord[], subcontractorWorkers: LedgerRecord[], slots: ScheduleSlot[]): ScheduleRow[] {
  const workers: ScheduleWorker[] = [
    ...employees.map(worker => ({ ...worker, worker_type: 'employee' as const, company_name: companyName })),
    ...subcontractorWorkers.map(worker => ({ ...worker, worker_type: 'subcontractor' as const, company_name: asText(worker.subcontractor_name) || '外注先' })),
  ]
  const groups = new Map<string, ScheduleWorker[]>()
  for (const worker of workers) groups.set(worker.company_name, [...(groups.get(worker.company_name) ?? []), worker])
  return [...groups.entries()].flatMap(([name, companyWorkers]) => companyWorkers.flatMap((worker, workerIndex) => slots.map((slot, slotIndex) => ({
    companyName: name,
    worker,
    slot: Number(slot.slot),
    companyRowSpan: companyWorkers.length * slots.length,
    firstInCompany: workerIndex === 0 && slotIndex === 0,
  }))))
}

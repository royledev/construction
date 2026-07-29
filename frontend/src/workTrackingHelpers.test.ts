import { describe, expect, it } from 'vitest'
import type { LedgerRecord, WorkTrackingRecord } from './api'
import { LatestRequest, canAddScheduleSlot, canCommitScope, canDeleteDailyReport, dailyScopeKey, dateLabels, groupDailyReports, nextDailyView, releaseBusyOperation, scheduleRows, scopedItems, upsertReport, viewForMonth } from './workTrackingHelpers'

const report = (id: number, date: string, extra: Record<string, unknown> = {}): WorkTrackingRecord => ({ id, revision: 1, report_date: date, ...extra })

describe('daily report helpers', () => {
  it('groups workers beneath their API company name and preserves row order', () => {
    const groups = groupDailyReports([
      report(2, '2026-07-02', { worker_type: 'employee', employee_id: 10, company_name: '自社株式会社', employee_name: '佐藤' }),
      report(1, '2026-07-01', { worker_type: 'employee', employee_id: 10, company_name: '自社株式会社', employee_name: '佐藤' }),
      report(3, '2026-07-01', { worker_type: 'subcontractor', company_name: '協力会社', subcontractor_worker_name: '田中' }),
    ])
    expect(groups.map(group => group.companyName)).toEqual(['自社株式会社', '協力会社'])
    expect(groups[0].workers[0].rows.map(row => row.id)).toEqual([1, 2])
  })

  it('does not merge different workers who share one displayed name', () => {
    const groups = groupDailyReports([
      report(1, '2026-07-01', { worker_type: 'employee', employee_id: 10, company_name: '自社株式会社', employee_name: '佐藤' }),
      report(2, '2026-07-01', { worker_type: 'employee', employee_id: 11, company_name: '自社株式会社', employee_name: '佐藤' }),
    ])
    expect(groups[0].workers).toHaveLength(2)
    expect(groups[0].workers.map(worker => worker.key)).toEqual(['employee:10', 'employee:11'])
  })

  it('only allows deletion when the worker-date has two or more loaded rows', () => {
    const one = report(1, '2026-07-01')
    const two = report(2, '2026-07-01')
    expect(canDeleteDailyReport([one], one)).toBe(false)
    expect(canDeleteDailyReport([one, two], one)).toBe(true)
  })

  it('upserts a POST row when an earlier GET already returned it', () => {
    const existing = report(1, '2026-07-01')
    const postResult = report(2, '2026-07-01', { work_description: 'POST結果' })
    expect(upsertReport([existing, postResult], postResult).map(row => row.id)).toEqual([1, 2])
  })

  it('keeps browse, input and preview states explicit', () => {
    expect(nextDailyView('mine', 'browse')).toBe('overview')
    expect(nextDailyView('overview', 'input')).toBe('mine')
    expect(nextDailyView('mine', 'preview')).toBe('preview')
    expect(viewForMonth('preview', '2026-07', '2026-08')).toBe('overview')
    expect(viewForMonth('mine', undefined, '2026-08')).toBe('mine')
  })

  it('accepts only the most recent asynchronous request generation', () => {
    const requests = new LatestRequest()
    const overview = requests.begin('2026-07:overview')
    const mine = requests.begin('2026-07:mine')
    expect(requests.accepts(overview, '2026-07:overview')).toBe(false)
    expect(requests.accepts(mine, '2026-07:mine')).toBe(true)
    expect(requests.accepts(mine, '2026-08:mine')).toBe(false)
    requests.invalidate()
    expect(requests.accepts(mine, '2026-07:mine')).toBe(false)
  })

  it('does not expose stale rows or mutation results outside their scope', () => {
    const mine = dailyScopeKey('2026-07', 'mine')
    const overview = dailyScopeKey('2026-07', 'overview')
    expect(scopedItems({ key: overview, items: [report(1, '2026-07-01')] }, mine)).toEqual([])
    expect(canCommitScope(mine, overview)).toBe(false)
    expect(canCommitScope(mine, mine)).toBe(true)
  })

  it('rejects a former scope for failed loads and slot POST completions', () => {
    const requests = new LatestRequest()
    const julyLoad = requests.begin('2026-07')
    // A failed July request must not surface its error after navigating August.
    expect(requests.accepts(julyLoad, '2026-08')).toBe(false)
    // Likewise, a July POST completion must not trigger a July reload later.
    expect(canCommitScope('2026-07', '2026-08')).toBe(false)
  })

  it('releases only the completed busy operation', () => {
    const oldOperation = { operationId: 1, key: '2026-07:mine' }
    const newOperation = { operationId: 2, key: '2026-08:mine' }
    expect(releaseBusyOperation([oldOperation, newOperation], 1)).toEqual([newOperation])
  })
})

describe('schedule helpers', () => {
  it('generates vertical company, worker and slot rows with a 31 day label contract', () => {
    const employees: LedgerRecord[] = [{ id: 1, full_name: '佐藤' }]
    const subcontractors: LedgerRecord[] = [{ id: 2, full_name: '田中', subcontractor_name: '協力会社' }]
    const rows = scheduleRows('自社株式会社', employees, subcontractors, [{ slot: 1 }, { slot: 2 }])
    expect(rows).toHaveLength(4)
    expect(rows[0]).toMatchObject({ companyName: '自社株式会社', companyRowSpan: 2, firstInCompany: true, slot: 1 })
    expect(rows[2]).toMatchObject({ companyName: '協力会社', companyRowSpan: 2, firstInCompany: true, slot: 1 })
    expect(dateLabels('2026-07')).toHaveLength(31)
    expect(dateLabels('2026-07')[0]).toEqual({ date: '2026-07-01', label: '7月1日' })
    expect(scheduleRows('自社株式会社', employees, [], [])).toEqual([])
    expect(scheduleRows('自社株式会社', employees, [], [{ slot: 1 }, { slot: 2 }, { slot: 3 }])[0].companyRowSpan).toBe(3)
    expect(canAddScheduleSlot('admin')).toBe(true)
    expect(canAddScheduleSlot('user')).toBe(false)
  })
})

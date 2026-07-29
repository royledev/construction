// @vitest-environment jsdom
import { cleanup, render, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DocumentDialog, DocumentWorkspace, LedgerTable, ProjectMembersTable, RelationshipEditor } from './App'
import { ledgerApi, type LedgerRecord, type SelectorChoices, type WorkTrackingRecord } from './api'
import { DailyGrid, DailyOverview } from './WorkTrackingWorkspace'

const choices: SelectorChoices = { clients: [], employees: [{ id: 1, employee_code: 'EMP-1', full_name: '田中', status: 'active' }], subcontractor_workers: [], projects: [] }

function expectActionHeaderLast(table: HTMLTableElement) {
  const headers = Array.from(table.tHead!.rows[0].cells)
  const actionHeader = headers.at(-1)!
  expect(actionHeader.textContent).toBe('操作')
  expect(actionHeader.tagName).toBe('TH')
  expect(actionHeader.getAttribute('scope')).toBe('col')
  return headers
}

function expectActionLast(table: HTMLTableElement) {
  const headers = expectActionHeaderLast(table)
  for (const row of Array.from(table.tBodies[0].rows)) {
    const cells = Array.from(row.cells)
    expect(cells).toHaveLength(headers.length)
    expect(cells.at(-1)!.classList.contains('action-column') || cells.at(-1)!.classList.contains('project-member-action')).toBe(true)
  }
}

function expectSingleColSpanRow(table: HTMLTableElement) {
  const rows = Array.from(table.tBodies[0].rows)
  expect(rows).toHaveLength(1)
  expect(rows[0].cells).toHaveLength(1)
  expect(rows[0].cells[0].colSpan).toBe(table.tHead!.rows[0].cells.length)
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('一覧テーブルの操作列', () => {
  it('keeps DocumentWorkspace and LedgerTable loading/empty colSpans aligned with their rightmost action headers', async () => {
    let resolveDocuments!: (value: { items: LedgerRecord[] }) => void
    const documents = new Promise<{ items: LedgerRecord[] }>(resolve => { resolveDocuments = resolve })
    vi.spyOn(ledgerApi, 'list').mockReturnValueOnce(documents)
    const documentView = render(<DocumentWorkspace title="見積" canEdit />)
    const documentTable = documentView.container.querySelector('table')!
    expectActionHeaderLast(documentTable)
    expectSingleColSpanRow(documentTable)
    resolveDocuments({ items: [] })
    await waitFor(() => expect(documentTable.tBodies[0].textContent).toContain('データがありません。'))
    expectSingleColSpanRow(documentTable)
    documentView.unmount()

    let resolveLedger!: (value: { items: LedgerRecord[] }) => void
    const ledgerItems = new Promise<{ items: LedgerRecord[] }>(resolve => { resolveLedger = resolve })
    vi.spyOn(ledgerApi, 'list').mockReturnValueOnce(ledgerItems)
    const ledger = { resource: 'clients', columns: [{ key: 'name', label: '得意先名' }], fields: [], initial: { id: 0 } }
    const ledgerView = render(<LedgerTable title="得意先" ledger={ledger} canEdit={false} />)
    const ledgerTable = ledgerView.container.querySelector('table')!
    expectActionHeaderLast(ledgerTable)
    expectSingleColSpanRow(ledgerTable)
    resolveLedger({ items: [] })
    await waitFor(() => expect(ledgerTable.tBodies[0].textContent).toContain('データがありません。'))
    expectSingleColSpanRow(ledgerTable)
  })

  it('places populated document and ledger row actions in the final cell', async () => {
    vi.spyOn(ledgerApi, 'list').mockResolvedValueOnce({ items: [{ id: 1, estimate_code: 'EST-1', issued_on: '2026-07-26' }] })
    const documentView = render(<DocumentWorkspace title="見積" canEdit />)
    await waitFor(() => expect(documentView.container.querySelector<HTMLTableRowElement>('tbody tr')!.cells.length).toBe(8))
    expectActionLast(documentView.container.querySelector('table')!)
    documentView.unmount()

    vi.spyOn(ledgerApi, 'list').mockResolvedValueOnce({ items: [{ id: 1, name: '得意先A' }] })
    const ledger = { resource: 'clients', columns: [{ key: 'name', label: '得意先名' }], fields: [], initial: { id: 0 } }
    const ledgerView = render(<LedgerTable title="得意先" ledger={ledger} canEdit={false} />)
    await waitFor(() => expect(ledgerView.container.querySelector<HTMLTableRowElement>('tbody tr')!.cells.length).toBe(2))
    expectActionLast(ledgerView.container.querySelector('table')!)
  })

  it('keeps document detail and relationship action cells last in read-only mode', async () => {
    const dataSource = {
      loadSelectors: vi.fn(async () => choices),
      loadDocument: vi.fn(async () => ({ item: { id: 1, project_id: 1, issued_on: '2026-07-26', status: 'draft', discount_amount: '0', lines: [{ item_name: '施工', quantity: '1', unit: '式', unit_price: '100', notes: '' }] } })),
      create: vi.fn(), update: vi.fn(),
    }
    const documentView = render(<DocumentDialog title="見積" resource="estimates" initial={{ id: 1, project_id: 1 }} canEdit={false} onClose={() => undefined} onSaved={() => undefined} dataSource={dataSource} />)
    await waitFor(() => expect(documentView.baseElement.querySelectorAll('table').length).toBe(1))
    expectActionLast(documentView.baseElement.querySelector('table')!)
    documentView.unmount()

    const relationshipView = render(<RelationshipEditor kind="worker" value={{ id: 1, workers: [{ id: 3, full_name: '佐藤' }] }} choices={choices} disabled setValue={() => undefined} />)
    expectActionLast(relationshipView.container.querySelector('table')!)
  })

  it('keeps project member action columns rightmost only while editable and preserves empty colSpan', () => {
    const view = render(<ProjectMembersTable title="自社従業員" memberType="employee" members={[]} choices={choices} disabled update={() => undefined} remove={() => undefined} />)
    const readOnlyTable = view.container.querySelector('table')!
    expect(readOnlyTable.tHead!.rows[0].textContent).not.toContain('操作')
    expectSingleColSpanRow(readOnlyTable)
    view.rerender(<ProjectMembersTable title="自社従業員" memberType="employee" members={[{ id: 1, employee_id: 1, notes: '読取専用メモ' }]} choices={choices} disabled update={() => undefined} remove={() => undefined} />)
    const populatedReadOnlyTable = view.container.querySelector('table')!
    const readOnlyHeaders = Array.from(populatedReadOnlyTable.tHead!.rows[0].cells)
    const readOnlyCells = Array.from(populatedReadOnlyTable.tBodies[0].rows[0].cells)
    expect(readOnlyHeaders.at(-1)!.textContent).toBe('備考')
    expect(readOnlyCells).toHaveLength(readOnlyHeaders.length)
    expect(readOnlyCells.at(-1)!.querySelector<HTMLInputElement | HTMLTextAreaElement>('input, textarea')?.value).toBe('読取専用メモ')
    expect(populatedReadOnlyTable.querySelector('.project-member-action, .action-column')).toBeNull()
    view.rerender(<ProjectMembersTable title="自社従業員" memberType="employee" members={[{ id: 1, employee_id: 1 }]} choices={choices} disabled={false} update={() => undefined} remove={() => undefined} />)
    expectActionLast(view.container.querySelector('table')!)
  })

  it('keeps daily overview and read-only daily grid action cells last', () => {
    const row: WorkTrackingRecord = { id: 1, revision: 1, report_date: '2026-07-01', employee_id: 1, employee_name: '田中', occupation: '大工', site_name: '本社', client_name: '得意先', started_at: '08:00', ended_at: '17:00', break_minutes: 60, work_description: '施工' }
    const overview = render(<DailyOverview rows={[row]} onPreview={() => undefined} />)
    expectActionLast(overview.container.querySelector('table')!)
    overview.unmount()
    const grid = render(<DailyGrid rows={[row]} setRows={() => undefined} scopeKey="2026-07:preview" isCurrentScope={() => true} onReload={() => undefined} readOnly onError={() => undefined} />)
    expectActionLast(grid.container.querySelector('table')!)
  })
})

// @vitest-environment jsdom
import { StrictMode } from 'react'
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DocumentDialog, type DocumentDialogDataSource } from './App'
import type { LedgerRecord, SelectorChoices } from './api'

type Deferred<T> = { promise: Promise<T>; resolve: (value: T) => void }

function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(next => { resolve = next })
  return { promise, resolve }
}

function documentDetail(typeKey: string, typeValue: string): LedgerRecord {
  return { id: 1, project_id: 1, issued_on: '2026-07-25', [typeKey]: typeValue, discount_amount: '0', lines: [{ item_name: '施工', quantity: '1', unit: '式', unit_price: '100', notes: '' }] }
}

afterEach(() => cleanup())

describe('DocumentDialog project selector hydration', () => {
  it.each([
    ['見積', 'estimates', 'status', 'draft'],
    ['請求', 'invoices', 'billing_type', 'individual'],
  ] as const)('keeps project_id while %s detail hydrates before selectors in StrictMode', async (title, resource, typeKey, typeValue) => {
    const selectors = deferred<SelectorChoices>()
    const detail = deferred<{ item: LedgerRecord }>()
    const saved: Array<{ resource: string; id: number; payload: LedgerRecord }> = []
    const dataSource: DocumentDialogDataSource = {
      loadSelectors: vi.fn(() => selectors.promise),
      loadDocument: vi.fn(() => detail.promise),
      create: vi.fn(),
      update: vi.fn(async (nextResource, id, payload) => { saved.push({ resource: nextResource, id, payload }) }),
    }
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    try {
      render(<StrictMode><DocumentDialog title={title} resource={resource} initial={{ id: 1, project_id: '' }} canEdit onClose={() => undefined} onSaved={() => undefined} dataSource={dataSource} /></StrictMode>)
      await act(async () => { detail.resolve({ item: { id: 1, project_id: 1, issued_on: '2026-07-25', [typeKey]: typeValue, lines: [{ item_name: '施工', quantity: 1, unit: '式', unit_price: 100, notes: '' }] } }) })
      await waitFor(() => expect(screen.getByText('ID: 1 / 現在の案件')).toBeTruthy())
      expect(warn.mock.calls.filter(([message]) => String(message).includes('out-of-range'))).toHaveLength(0)

      await act(async () => { selectors.resolve({ clients: [], employees: [], subcontractor_workers: [], projects: [{ id: 1, project_code: 'PRJ-000001', site_name: '本社改修' }] }) })
      await waitFor(() => expect(screen.getByText('PRJ-000001 / 本社改修')).toBeTruthy())
      expect(screen.queryByText('ID: 1 / 現在の案件')).toBeNull()

      fireEvent.click(screen.getByRole('button', { name: '保存' }))
      await waitFor(() => expect(saved).toHaveLength(1))
      expect(saved[0]).toMatchObject({ resource, id: 1, payload: { project_id: 1 } })
      expect(warn.mock.calls.filter(([message]) => String(message).includes('out-of-range'))).toHaveLength(0)
    } finally {
      warn.mockRestore()
    }
  })

  it.each([
    ['見積', 'estimates', 'status', 'draft'],
    ['請求', 'invoices', 'billing_type', 'individual'],
  ] as const)('%s recalculates stale API amounts immediately and submits only writable values', async (title, resource, typeKey, typeValue) => {
    const saved: LedgerRecord[] = []
    const dataSource: DocumentDialogDataSource = {
      loadSelectors: vi.fn(async () => ({ clients: [], employees: [], subcontractor_workers: [], projects: [{ id: 1, project_code: 'PRJ-000001', site_name: '本社改修' }] })),
      loadDocument: vi.fn(async () => ({ item: {
        id: 1, project_id: 1, issued_on: '2026-07-25', [typeKey]: typeValue,
        amount: 888, subtotal: 888, discount_amount: 0, pre_tax: 888, tax_amount: 888, total: 888,
        estimate_code: 'EST-ignored', invoice_code: 'INV-ignored', project_code: 'PRJ-ignored', created_at: '2026-07-25',
        lines: [{ id: 7, item_name: '施工', quantity: '1', unit: '式', unit_price: '100', notes: '', amount: 888 }],
      } })),
      create: vi.fn(),
      update: vi.fn(async (_nextResource, _id, payload) => { saved.push(payload) }),
    }
    render(<DocumentDialog title={title} resource={resource} initial={{ id: 1, project_id: 1 }} canEdit onClose={() => undefined} onSaved={() => undefined} dataSource={dataSource} />)
    await waitFor(() => expect(screen.getByDisplayValue('施工')).toBeTruthy())
    expect(screen.getByRole('combobox', { name: '案件' })).toBeTruthy()
    expect(screen.getByRole('combobox', { name: title === '見積' ? 'ステータス' : '請求区分' })).toBeTruthy()
    expect(screen.getAllByText('¥100').length).toBeGreaterThan(0)
    expect(screen.queryByText('¥888')).toBeNull()

    const quantity = screen.getByRole('spinbutton', { name: '明細1数量' })
    const unitPrice = screen.getByRole('spinbutton', { name: '明細1単価' })
    fireEvent.change(quantity, { target: { value: '0' } })
    expect(screen.getByText(/数量は0より大きく/)).toBeTruthy()
    expect(screen.getByRole('button', { name: '保存' }).hasAttribute('disabled')).toBe(true)
    fireEvent.change(quantity, { target: { value: '2' } })
    fireEvent.change(unitPrice, { target: { value: '100.5' } })
    expect(screen.getAllByText('¥201').length).toBeGreaterThan(0)
    expect(screen.getByRole('button', { name: 'PDFをダウンロード' }).hasAttribute('disabled')).toBe(true)
    expect(screen.getByText('保存後にPDFへ反映されます。')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: '明細を追加' }))
    await waitFor(() => expect(screen.getByLabelText('明細2項目名')).toBeTruthy())
    expect(screen.getByText(`${title === '見積' ? '御見積合計' : '御請求合計'} ¥221`)).toBeTruthy()
    fireEvent.change(screen.getByRole('spinbutton', { name: '明細2数量' }), { target: { value: '2' } })
    fireEvent.change(screen.getByRole('spinbutton', { name: '明細2単価' }), { target: { value: '5' } })
    expect(screen.getByText(`${title === '見積' ? '御見積合計' : '御請求合計'} ¥232`)).toBeTruthy()
    fireEvent.change(screen.getByRole('textbox', { name: '明細2項目名' }), { target: { value: '追加分' } })
    fireEvent.click(screen.getByRole('button', { name: '明細2を削除' }))
    expect(screen.getByText(`${title === '見積' ? '御見積合計' : '御請求合計'} ¥221`)).toBeTruthy()
    fireEvent.change(screen.getByRole('spinbutton', { name: '値引き（円）' }), { target: { value: '0.50' } })
    expect(screen.getByText('値引き ¥1')).toBeTruthy()
    expect(screen.getByText(`${title === '見積' ? '御見積合計' : '御請求合計'} ¥220`)).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => expect(saved).toHaveLength(1))
    expect(saved[0]).toMatchObject({ project_id: 1, [typeKey]: typeValue, discount_amount: '0.5', lines: [{ item_name: '施工', quantity: '2', unit_price: '100.5' }] })
    for (const key of ['id', 'amount', 'subtotal', 'pre_tax', 'tax_amount', 'total', 'estimate_code', 'invoice_code', 'project_code', 'created_at']) expect(saved[0]).not.toHaveProperty(key)
  })

  it.each([
    ['見積', 'estimates', 'status', 'draft'],
    ['請求', 'invoices', 'billing_type', 'individual'],
  ] as const)('%s ignores a cancelled StrictMode detail response and locks editing until the current response', async (title, resource, typeKey, typeValue) => {
    const first = deferred<{ item: LedgerRecord }>()
    const second = deferred<{ item: LedgerRecord }>()
    const dataSource: DocumentDialogDataSource = {
      loadSelectors: vi.fn(async () => ({ clients: [], employees: [], subcontractor_workers: [], projects: [{ id: 1, project_code: 'PRJ-000001', site_name: '本社改修' }] })),
      loadDocument: vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise),
      create: vi.fn(), update: vi.fn(),
    }
    render(<StrictMode><DocumentDialog title={title} resource={resource} initial={{ id: 1, project_id: 1 }} canEdit onClose={() => undefined} onSaved={() => undefined} dataSource={dataSource} /></StrictMode>)
    await waitFor(() => expect(dataSource.loadDocument).toHaveBeenCalledTimes(2))
    expect(screen.getByLabelText('請負者会社名').hasAttribute('disabled')).toBe(true)
    await act(async () => { second.resolve({ item: { ...documentDetail(typeKey, typeValue), contractor_name: '最新' } }) })
    await waitFor(() => expect(screen.getByDisplayValue('最新')).toBeTruthy())
    fireEvent.change(screen.getByLabelText('請負者会社名'), { target: { value: '編集中' } })
    await act(async () => { first.resolve({ item: { ...documentDetail(typeKey, typeValue), contractor_name: '古い応答' } }) })
    expect(screen.getByDisplayValue('編集中')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'PDFをダウンロード' }).hasAttribute('disabled')).toBe(true)
  })

  it.each([
    ['見積', 'estimates', 'status', 'draft'],
    ['請求', 'invoices', 'billing_type', 'individual'],
  ] as const)('%s disables every document mutation while a deferred save is in flight', async (title, resource, typeKey, typeValue) => {
    const saving = deferred<unknown>()
    const onSaved = vi.fn()
    const dataSource: DocumentDialogDataSource = {
      loadSelectors: vi.fn(async () => ({ clients: [], employees: [], subcontractor_workers: [], projects: [{ id: 1, project_code: 'PRJ-000001', site_name: '本社改修' }] })),
      loadDocument: vi.fn(async () => ({ item: documentDetail(typeKey, typeValue) })),
      create: vi.fn(), update: vi.fn(() => saving.promise),
    }
    render(<DocumentDialog title={title} resource={resource} initial={{ id: 1, project_id: 1 }} canEdit onClose={() => undefined} onSaved={onSaved} dataSource={dataSource} />)
    await waitFor(() => expect(screen.getByDisplayValue('施工')).toBeTruthy())
    const contractor = screen.getByLabelText('請負者会社名') as HTMLInputElement
    fireEvent.change(contractor, { target: { value: '保存対象' } })
    fireEvent.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => expect(dataSource.update).toHaveBeenCalledTimes(1))
    expect(contractor.disabled).toBe(true)
    expect(screen.getByRole('button', { name: '明細を追加' }).hasAttribute('disabled')).toBe(true)
    expect(screen.getByRole('button', { name: 'PDFをダウンロード' }).hasAttribute('disabled')).toBe(true)
    fireEvent.change(contractor, { target: { value: '保存後に失われる編集' } })
    expect(contractor.value).toBe('保存対象')
    await act(async () => { saving.resolve(undefined) })
    await waitFor(() => expect(onSaved).toHaveBeenCalledTimes(1))
  })

  it('blocks an oversized whitelist JSON body before calling the save API', async () => {
    const update = vi.fn()
    const lines = Array.from({ length: 33 }, (_, index) => ({ item_name: `施工${index + 1}`, quantity: '1', unit: '式', unit_price: '1', notes: 'x'.repeat(65_535) }))
    const dataSource: DocumentDialogDataSource = {
      loadSelectors: vi.fn(async () => ({ clients: [], employees: [], subcontractor_workers: [], projects: [{ id: 1, project_code: 'PRJ-000001', site_name: '本社改修' }] })),
      loadDocument: vi.fn(async () => ({ item: { ...documentDetail('status', 'draft'), lines } })),
      create: vi.fn(), update,
    }
    render(<DocumentDialog title="見積" resource="estimates" initial={{ id: 1, project_id: 1 }} canEdit onClose={() => undefined} onSaved={() => undefined} dataSource={dataSource} />)
    await waitFor(() => expect(screen.getByText(/入力内容が大きすぎます/)).toBeTruthy())
    expect(screen.getByRole('button', { name: '保存' }).hasAttribute('disabled')).toBe(true)
    fireEvent.click(screen.getByRole('button', { name: '保存' }))
    expect(update).not.toHaveBeenCalled()
  })
})

import { describe, expect, it } from 'vitest'
import { addDocumentLine, choicesWithCurrent, dateInputValue, documentLineRowKey, documentLinesPayload, documentPayloadBytes, documentSavePayload, documentTypeLabel, documentTypeOptions, MAX_DOCUMENT_JSON_BYTES, normalizeDocumentForm, validateDocumentForm } from './documentForm'

describe('document form normalization', () => {
  it('uses a YYYY-MM-DD date and document_type fallback for list rows', () => {
    expect(dateInputValue('2026-07-25T00:00:00Z')).toBe('2026-07-25')
    expect(normalizeDocumentForm('estimates', { issued_on: '2026-07-25T00:00:00Z', document_type: 'confirmed' })).toMatchObject({ issued_on: '2026-07-25', status: 'confirmed' })
    expect(normalizeDocumentForm('invoices', { issued_on: '2026-07-25T00:00:00Z', document_type: 'closing' })).toMatchObject({ issued_on: '2026-07-25', billing_type: 'closing' })
  })

  it('preserves the canonical API write key for existing and new documents', () => {
    const readOnlyValues = { id: 7, estimate_code: 'EST-123456', document_type: 'confirmed', amount: 999, subtotal: 999, pre_tax: 999, tax_amount: 999, total: 999, created_at: '2026-07-01', project_code: 'PRJ-1' }
    expect(documentSavePayload('estimates', { ...readOnlyValues, project_id: '2', status: 'draft', issued_on: '2026-07-01T00:00:00Z', discount_amount: '1.20', lines: [{ id: 4, amount: 999, item_name: '施工', quantity: '1.005', unit: '式', unit_price: '100.00', notes: '' }] })).toEqual({ project_id: 2, status: 'draft', issued_on: '2026-07-01', contractor_name: '', contractor_representative: '', contractor_address: '', discount_amount: '1.2', lines: [{ item_name: '施工', quantity: '1.005', unit: '式', unit_price: '100', notes: '' }] })
    expect(documentSavePayload('invoices', { project_id: 3, invoice_code: 'INV-123456', issued_on: '2026-07-02', discount_amount: 0, lines: [{ item_name: '施工', quantity: '1', unit: '', unit_price: '0', notes: '' }] })).toMatchObject({ project_id: 3, issued_on: '2026-07-02', billing_type: 'individual', discount_amount: '0', lines: [{ item_name: '施工' }] })
  })

  it('provides direct Select children for each document type', () => {
    expect(documentTypeOptions('estimates')).toEqual([['draft', '未確定'], ['confirmed', '確定']])
    expect(documentTypeOptions('invoices')).toEqual([['closing', '締め'], ['individual', '個別']])
  })

  it('maps every document type to its Japanese list label and preserves unknown values', () => {
    expect(documentTypeLabel('estimates', 'draft')).toBe('未確定')
    expect(documentTypeLabel('estimates', 'confirmed')).toBe('確定')
    expect(documentTypeLabel('invoices', 'closing')).toBe('締め')
    expect(documentTypeLabel('invoices', 'individual')).toBe('個別')
    expect(documentTypeLabel('estimates', 'archived')).toBe('archived')
    expect(documentTypeLabel('invoices', '')).toBe('—')
  })

  it('keeps document line keys stable after deleting a previous unsaved line and omits UI keys from the API payload', () => {
    const lines = addDocumentLine(addDocumentLine([], { item_name: '先頭', quantity: '1', unit: '式', unit_price: '100', notes: '' }), { item_name: '残す', quantity: '2', unit: '本', unit_price: '200', notes: '' })
    expect(documentLineRowKey(lines[1])).toBe('document-line-new-2')
    expect(documentLineRowKey(lines.slice(1)[0])).toBe('document-line-new-2')
    expect(documentLineRowKey({ id: 44, _client_key: 'document-line-new-9' })).toBe('document-line-44')
    expect(documentLinesPayload(lines)).toEqual([
      { item_name: '先頭', quantity: '1', unit: '式', unit_price: '100', notes: '' },
      { item_name: '残す', quantity: '2', unit: '本', unit_price: '200', notes: '' },
    ])
  })

  it('includes a selected project fallback only until its selector choice is loaded', () => {
    const fallback = (id: number) => ({ id, project_code: `ID: ${id}`, site_name: '現在の案件' })
    expect(choicesWithCurrent([], 1, fallback)).toEqual([{ choice: fallback(1), isFallback: true }])
    expect(choicesWithCurrent([{ id: 1, project_code: 'PRJ-000001', site_name: '本社改修' }], 1, fallback)).toEqual([
      { choice: { id: 1, project_code: 'PRJ-000001', site_name: '本社改修' }, isFallback: false },
    ])
    expect(choicesWithCurrent([{ id: 1, project_code: 'PRJ-000001', site_name: '本社改修' }], '', fallback)).toHaveLength(1)
    expect(choicesWithCurrent([], 0, fallback)).toEqual([])
  })

  it('mirrors server validation for required fields, line count, and database text limits', () => {
    const valid = { project_id: 1, issued_on: '2026-07-26', status: 'draft', discount_amount: '0', contractor_name: 'あ'.repeat(160), contractor_representative: 'い'.repeat(120), contractor_address: 'う'.repeat(255), lines: [{ item_name: 'え'.repeat(160), quantity: '1', unit: 'お'.repeat(24), unit_price: '0', notes: 'n'.repeat(65_535) }] }
    expect(validateDocumentForm('estimates', valid)).toBeNull()
    expect(validateDocumentForm('estimates', { ...valid, issued_on: '' })).toBe('発行日は必須です。')
    expect(validateDocumentForm('estimates', { ...valid, issued_on: '2026-02-29' })).toBe('発行日が不正です。')
    expect(validateDocumentForm('estimates', { ...valid, lines: [{ ...valid.lines[0], item_name: '  ' }] })).toBe('明細の入力内容が不正です。')
    expect(validateDocumentForm('estimates', { ...valid, lines: Array.from({ length: 501 }, () => valid.lines[0]) })).toBe('明細は500行まで入力できます。')
    expect(validateDocumentForm('estimates', { ...valid, contractor_name: 'あ'.repeat(161) })).toBe('帳票発行者の文字数または入力形式が不正です。')
    expect(validateDocumentForm('estimates', { ...valid, lines: [{ ...valid.lines[0], notes: 'n'.repeat(65_536) }] })).toBe('明細の入力内容が不正です。')
  })

  it('measures the actual whitelist JSON body in UTF-8 and enforces the backend 2MiB boundary', () => {
    const line = { item_name: '施工', quantity: '1', unit: '式', unit_price: '1', notes: '' }
    const base = { project_id: 1, issued_on: '2026-07-26', status: 'draft', discount_amount: '0', lines: Array.from({ length: 500 }, () => ({ ...line })) }
    const baseBytes = documentPayloadBytes(documentSavePayload('estimates', base))
    let remaining = MAX_DOCUMENT_JSON_BYTES - baseBytes
    const exact = { ...base, lines: base.lines.map(current => {
      const length = Math.min(65_535, remaining)
      remaining -= length
      return { ...current, notes: 'x'.repeat(length) }
    }) }
    const payload = documentSavePayload('estimates', exact)
    expect(remaining).toBe(0)
    expect(documentPayloadBytes(payload)).toBe(MAX_DOCUMENT_JSON_BYTES)
    const expandable = exact.lines.findIndex(current => current.notes.length < 65_535)
    const tooLarge = { ...exact, lines: exact.lines.map((current, index) => index === expandable ? { ...current, notes: `${current.notes}x` } : current) }
    expect(() => documentSavePayload('estimates', tooLarge)).toThrow('入力内容が大きすぎます。')
    expect(() => documentSavePayload('estimates', { ...base, lines: base.lines.map(current => ({ ...current, notes: 'x'.repeat(65_535) })) })).toThrow('入力内容が大きすぎます。')
  })
})

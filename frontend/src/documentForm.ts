import { calculateDocumentAmounts, MAX_DOCUMENT_DECIMAL_CENTS, MAX_QUANTITY_MILLI, normalizeDocumentDecimal } from './documentAmounts'

export type DocumentResource = 'estimates' | 'invoices'
export type DocumentLineInput = Record<string, unknown>
export const MAX_DOCUMENT_JSON_BYTES = 2 << 20

const typeKeyFor = (resource: DocumentResource) => resource === 'estimates' ? 'status' : 'billing_type'
const defaultTypeFor = (resource: DocumentResource) => resource === 'estimates' ? 'draft' : 'individual'

export function documentTypeOptions(resource: DocumentResource): Array<[string, string]> {
  return resource === 'estimates' ? [['draft', '未確定'], ['confirmed', '確定']] : [['closing', '締め'], ['individual', '個別']]
}

export function documentTypeLabel(resource: DocumentResource, value: unknown): string {
  const current = String(value ?? '')
  return documentTypeOptions(resource).find(([key]) => key === current)?.[1] ?? (current || '—')
}

export function dateInputValue(value: unknown): string {
  const match = String(value ?? '').match(/^\d{4}-\d{2}-\d{2}/)
  return match?.[0] ?? ''
}

export function documentTypeValue(resource: DocumentResource, value: Record<string, unknown>): string {
  const typeKey = typeKeyFor(resource)
  return String(value[typeKey] ?? value.document_type ?? defaultTypeFor(resource))
}

// List responses can carry document_type while detail responses carry the API
// write key. Normalize both shapes before rendering or submitting the form.
export function normalizeDocumentForm(resource: DocumentResource, value: Record<string, unknown>): Record<string, unknown> {
  const typeKey = typeKeyFor(resource)
  return { ...value, issued_on: dateInputValue(value.issued_on), [typeKey]: documentTypeValue(resource, value) }
}

const runeLength = (value: string) => Array.from(value.trim()).length
const byteLength = (value: string) => new TextEncoder().encode(value.trim()).length
const optionalTextWithin = (value: unknown, maximumRunes: number, maximumBytes = 0) => value === undefined || value === null || typeof value === 'string' && runeLength(value) <= maximumRunes && (maximumBytes === 0 || byteLength(value) <= maximumBytes)
const validDate = (value: unknown) => {
  if (typeof value !== 'string' || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return false
  const [year, month, day] = value.split('-').map(Number)
  const date = new Date(Date.UTC(year, month - 1, day))
  return date.getUTCFullYear() === year && date.getUTCMonth() === month - 1 && date.getUTCDate() === day
}
const validProjectID = (value: unknown) => (typeof value === 'number' && Number.isInteger(value) || typeof value === 'string' && /^[1-9]\d*$/.test(value))

// Mirrors validateAndCalculateDocument without imposing client-only policy.
export function validateDocumentForm(resource: DocumentResource, value: Record<string, unknown>): string | null {
  if (!optionalTextWithin(value.contractor_name, 160) || !optionalTextWithin(value.contractor_representative, 120) || !optionalTextWithin(value.contractor_address, 255)) return '帳票発行者の文字数または入力形式が不正です。'
  if (typeof value.issued_on !== 'string' || value.issued_on.trim() === '') return '発行日は必須です。'
  if (!validDate(value.issued_on)) return '発行日が不正です。'
  if (!validProjectID(value.project_id)) return '案件を指定してください。'
  if (resource === 'estimates' && !['draft', 'confirmed'].includes(documentTypeValue(resource, value))) return '見積ステータスが不正です。'
  if (resource === 'invoices' && !['closing', 'individual'].includes(documentTypeValue(resource, value))) return '請求区分が不正です。'
  if (!Array.isArray(value.lines) || value.lines.length === 0) return '明細を1行以上入力してください。'
  if (value.lines.length > 500) return '明細は500行まで入力できます。'
  for (const line of value.lines) {
    if (line === null || typeof line !== 'object') return '明細の入力内容が不正です。'
    const row = line as DocumentLineInput
    if (typeof row.item_name !== 'string' || row.item_name.trim() === '' || !optionalTextWithin(row.item_name, 160) || !optionalTextWithin(row.unit, 24) || !optionalTextWithin(row.notes, Number.MAX_SAFE_INTEGER, 65_535)) return '明細の入力内容が不正です。'
  }
  const calculation = calculateDocumentAmounts(value.lines as Array<{ quantity: unknown; unit_price: unknown }>, value.discount_amount ?? '0')
  return calculation.ok ? null : calculation.error
}

export function documentPayloadBytes(payload: unknown): number {
  return new TextEncoder().encode(JSON.stringify(payload)).byteLength
}

export function documentSavePayload(resource: DocumentResource, value: Record<string, unknown>): Record<string, unknown> {
  const validationError = validateDocumentForm(resource, { ...value, issued_on: dateInputValue(value.issued_on) })
  if (validationError) throw new Error(validationError)
  const typeKey = typeKeyFor(resource)
  const projectID = Number(value.project_id)
  if (!Number.isInteger(projectID) || projectID <= 0) throw new Error('案件を指定してください。')
  const discount = normalizeDocumentDecimal(value.discount_amount ?? '0', 2, MAX_DOCUMENT_DECIMAL_CENTS)
  if (discount === null) throw new Error('値引きは0以上の数値で入力してください。')
  const rawLines = Array.isArray(value.lines) ? value.lines.filter((line): line is DocumentLineInput => line !== null && typeof line === 'object') : []
  const payload = {
    project_id: projectID,
    issued_on: dateInputValue(value.issued_on),
    [typeKey]: documentTypeValue(resource, value),
    contractor_name: String(value.contractor_name ?? ''),
    contractor_representative: String(value.contractor_representative ?? ''),
    contractor_address: String(value.contractor_address ?? ''),
    discount_amount: discount,
    lines: documentLinesPayload(rawLines),
  }
  if (documentPayloadBytes(payload) > MAX_DOCUMENT_JSON_BYTES) throw new Error('入力内容が大きすぎます。')
  return payload
}

export type ChoiceWithFallback<T> = { choice: T; isFallback: boolean }

export function choicesWithCurrent<T extends { id: number }>(choices: T[], currentID: unknown, fallback: (id: number) => T): ChoiceWithFallback<T>[] {
  const id = Number(currentID)
  const normalized = choices.map(choice => ({ choice, isFallback: false }))
  return Number.isInteger(id) && id > 0 && !choices.some(choice => choice.id === id) ? [{ choice: fallback(id), isFallback: true }, ...normalized] : normalized
}

export function documentLineRowKey(line: DocumentLineInput): string {
  const id = Number(line.id)
  if (Number.isInteger(id) && id > 0) return `document-line-${id}`
  return String(line._client_key ?? 'document-line-unsaved')
}

export function withDocumentLineClientKeys<T extends DocumentLineInput>(lines: T[]): T[] {
  return lines.map((line, index) => line._client_key || Number(line.id) > 0 ? line : { ...line, _client_key: `document-line-new-${index + 1}` })
}

export function addDocumentLine<T extends DocumentLineInput>(lines: T[], line: T): T[] {
  const largest = lines.reduce((max, current) => {
    const match = /^document-line-new-(\d+)$/.exec(String(current._client_key ?? ''))
    return match ? Math.max(max, Number(match[1])) : max
  }, 0)
  return [...lines, { ...line, _client_key: `document-line-new-${largest + 1}` }]
}

export function documentLinesPayload(lines: DocumentLineInput[]): Array<{ item_name: string; quantity: string; unit: string; unit_price: string; notes: string }> {
  return lines.map(line => {
    const quantity = normalizeDocumentDecimal(line.quantity, 3, MAX_QUANTITY_MILLI)
    const unitPrice = normalizeDocumentDecimal(line.unit_price, 2, MAX_DOCUMENT_DECIMAL_CENTS)
    if (quantity === null || unitPrice === null) throw new Error('数量または単価の入力形式が不正です。')
    return { item_name: String(line.item_name ?? ''), quantity, unit: String(line.unit ?? ''), unit_price: unitPrice, notes: String(line.notes ?? '') }
  })
}

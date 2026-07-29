export type DocumentAmountLine = { quantity: unknown; unit_price: unknown }

export type DocumentAmounts = {
  amounts: bigint[]
  subtotal: bigint
  discount: bigint
  preTax: bigint
  tax: bigint
  total: bigint
}

export type DocumentAmountResult = { ok: true; value: DocumentAmounts } | { ok: false; error: string }

export const MAX_QUANTITY_MILLI = 99_999_999_999_999n
export const MAX_DOCUMENT_DECIMAL_CENTS = 99_999_999_999_999n
export const MAX_DOCUMENT_YEN = 1_000_000_000_000n

type FixedDecimal = { scaled: bigint; normalized: string }

// This intentionally accepts only ordinary non-negative decimal notation.  Do
// not coerce through Number: it would accept exponent notation and lose cents.
export function fixedDecimal(value: unknown, places: number, maximum: bigint): FixedDecimal | null {
  if (typeof value !== 'string' && typeof value !== 'number') return null
  const text = String(value).trim()
  const match = /^(\d+)(?:\.(\d*))?$/.exec(text)
  if (!match || (match[2]?.length ?? 0) > places) return null
  const whole = match[1].replace(/^0+(?=\d)/, '')
  const fraction = (match[2] ?? '').padEnd(places, '0')
  const scaled = BigInt(whole) * (10n ** BigInt(places)) + BigInt(fraction || '0')
  if (scaled > maximum) return null
  const significantFraction = fraction.replace(/0+$/, '')
  return { scaled, normalized: significantFraction ? `${whole}.${significantFraction}` : whole }
}

export function normalizeDocumentDecimal(value: unknown, places: number, maximum = MAX_DOCUMENT_DECIMAL_CENTS): string | null {
  return fixedDecimal(value, places, maximum)?.normalized ?? null
}

export function calculateDocumentAmounts(lines: DocumentAmountLine[], discountValue: unknown): DocumentAmountResult {
  const amounts: bigint[] = []
  let subtotal = 0n
  for (const line of lines) {
    const quantity = fixedDecimal(line.quantity, 3, MAX_QUANTITY_MILLI)
    const unitPrice = fixedDecimal(line.unit_price, 2, MAX_DOCUMENT_DECIMAL_CENTS)
    if (!quantity || !unitPrice || quantity.scaled <= 0n) {
      return { ok: false, error: '数量は0より大きく、単価は0以上の数値で入力してください。' }
    }
    const amount = (quantity.scaled * unitPrice.scaled + 50_000n) / 100_000n
    if (amount > MAX_DOCUMENT_YEN || subtotal > MAX_DOCUMENT_YEN - amount) {
      return { ok: false, error: '合計金額が大きすぎます。' }
    }
    amounts.push(amount)
    subtotal += amount
  }
  const discountCents = fixedDecimal(discountValue, 2, MAX_DOCUMENT_DECIMAL_CENTS)
  if (!discountCents) return { ok: false, error: '値引きは0以上の数値で入力してください。' }
  const discount = (discountCents.scaled + 50n) / 100n
  if (discount > subtotal) return { ok: false, error: '値引きは明細合計以下で入力してください。' }
  const preTax = subtotal - discount
  const tax = (preTax + 5n) / 10n
  return { ok: true, value: { amounts, subtotal, discount, preTax, tax, total: preTax + tax } }
}

export function documentYen(value: bigint): string {
  return `¥${value.toLocaleString('ja-JP')}`
}

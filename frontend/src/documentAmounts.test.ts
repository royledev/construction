import { describe, expect, it } from 'vitest'
import { calculateDocumentAmounts, fixedDecimal, MAX_DOCUMENT_DECIMAL_CENTS, MAX_QUANTITY_MILLI } from './documentAmounts'

const line = (quantity: unknown, unit_price: unknown) => ({ quantity, unit_price })

describe('document amounts', () => {
  it('uses fixed-point half-up rounding for quantities and each line before subtotaling', () => {
    const result = calculateDocumentAmounts([line('1.005', '100'), line('0.004', '100'), line('0.005', '100')], '0')
    expect(result).toMatchObject({ ok: true })
    if (!result.ok) return
    expect(result.value.amounts).toEqual([101n, 0n, 1n])
    expect(result.value.subtotal).toBe(102n)
  })

  it('rounds a fractional discount and tax at their required half boundaries', () => {
    const lessThanHalf = calculateDocumentAmounts([line('1', '5')], '0.49')
    const half = calculateDocumentAmounts([line('1', '5')], '0.50')
    expect(lessThanHalf).toMatchObject({ ok: true })
    expect(half).toMatchObject({ ok: true })
    if (!lessThanHalf.ok || !half.ok) return
    expect(lessThanHalf.value.discount).toBe(0n)
    expect(half.value.discount).toBe(1n)
    expect(calculateDocumentAmounts([line('1', '4')], '0')).toMatchObject({ ok: true, value: { tax: 0n } })
    expect(calculateDocumentAmounts([line('1', '5')], '0')).toMatchObject({ ok: true, value: { tax: 1n } })
  })

  it('rejects non-fixed decimals, negatives, precision overflow, database boundaries and excess discount', () => {
    for (const value of ['', '-1', '1e2', '1.0001', '100000000000.000']) expect(calculateDocumentAmounts([line(value, '1')], '0').ok).toBe(false)
    expect(calculateDocumentAmounts([line('1', '1.001')], '0').ok).toBe(false)
    expect(calculateDocumentAmounts([line('1', '1000000000000.00')], '0').ok).toBe(false)
    expect(calculateDocumentAmounts([line('1', '1')], '1.50').ok).toBe(false)
    expect(fixedDecimal('99999999999.999', 3, MAX_QUANTITY_MILLI)?.scaled).toBe(MAX_QUANTITY_MILLI)
    expect(fixedDecimal('999999999999.99', 2, MAX_DOCUMENT_DECIMAL_CENTS)?.scaled).toBe(MAX_DOCUMENT_DECIMAL_CENTS)
  })
})

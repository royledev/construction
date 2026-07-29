import { describe, expect, it } from 'vitest'
import { displayLedgerValue } from './ledgerDisplay'

describe('displayLedgerValue', () => {
  it('keeps login IDs raw while formatting account codes by column', () => {
    expect(displayLedgerValue('login_id', 'admin')).toBe('admin')
    expect(displayLedgerValue('role', 'admin')).toBe('管理者')
    expect(displayLedgerValue('worker_type', 'subcontractor')).toBe('外注先')
  })

  it('keeps status labels without translating arbitrary values', () => {
    expect(displayLedgerValue('status', 'active')).toBe('有効・在籍')
    expect(displayLedgerValue('login_id', 'active')).toBe('active')
  })
})

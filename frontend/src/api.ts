export type User = { id: number; employeeId?: number; subcontractorWorkerId?: number; workerType: 'employee' | 'subcontractor'; workerId: number; loginId: string; displayName: string; role: 'admin' | 'user' }
const apiBase = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080/api/v1'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBase}${path}`, { credentials: 'include', headers: { 'Content-Type': 'application/json', ...init?.headers }, ...init })
  if (!response.ok) { const body = await response.json().catch(() => ({})); throw new Error(body.error ?? '通信に失敗しました。') }
  return response.status === 204 ? undefined as T : response.json() as Promise<T>
}

export const authApi = {
  login: (loginId: string, password: string) => request<{ user: User }>('/auth/login', { method: 'POST', body: JSON.stringify({ loginId, password }) }),
  logout: () => request<void>('/auth/logout', { method: 'POST' }),
  me: () => request<{ user: User }>('/auth/me'),
}

export type LedgerRecord = Record<string, unknown> & { id: number }
export type ClientSelector = { id: number; client_code: string; name: string; status: 'active' | 'inactive' }
export type EmployeeSelector = { id: number; employee_code: string; full_name: string; status: 'active' | 'retired' }
export type ProjectSelector = { id: number; project_code: string; site_name: string }
export type SubcontractorWorkerSelector = { id: number; full_name: string; subcontractor_code: string; subcontractor_name: string; subcontractor_status: 'active' | 'inactive' }
export type SelectorChoices = { clients: ClientSelector[]; employees: EmployeeSelector[]; subcontractor_workers: SubcontractorWorkerSelector[]; projects: ProjectSelector[] }
export type AccountWorkerChoices = { employees: EmployeeSelector[]; subcontractor_workers: SubcontractorWorkerSelector[] }
export const ledgerApi = {
  list: (resource: string, query = '') => request<{ items: LedgerRecord[] }>(`/${resource}${query}`),
  get: (resource: string, id: number) => request<{ item: LedgerRecord }>(`/${resource}/${id}`),
  create: (resource: string, data: Record<string, unknown>) => request<{ item: LedgerRecord }>(`/${resource}`, { method: 'POST', body: JSON.stringify(data) }),
  update: (resource: string, id: number, data: Record<string, unknown>) => request<{ item: LedgerRecord }>(`/${resource}/${id}`, { method: 'PATCH', body: JSON.stringify(data) }),
  remove: (resource: string, id: number) => request<void>(`/${resource}/${id}`, { method: 'DELETE' }),
}

export const selectorApi = {
  list: () => request<SelectorChoices>('/selectors'),
  accountWorkerChoices: () => request<AccountWorkerChoices>('/account-worker-choices'),
}

export const documentApi = {
  downloadPdf: async (resource: 'estimates' | 'invoices', id: number) => {
    const response = await fetch(`${apiBase}/${resource}/${id}/pdf`, { credentials: 'include' })
    if (!response.ok) { const body = await response.json().catch(() => ({})); throw new Error(body.error ?? 'PDFを生成できませんでした。') }
    const url = URL.createObjectURL(await response.blob())
    const link = document.createElement('a'); link.href = url; link.download = `${resource}-${id}.pdf`; link.click()
    URL.revokeObjectURL(url)
  },
}

export type WorkTrackingRecord = LedgerRecord & { revision: number; report_date?: string; schedule_date?: string; parent_report_id?: number | null; employee_id?: number; slot?: number; master_item_id?: number | null }
export type ScheduleSlot = { slot: number }
export type ScheduleResponse = {
  company_name: string
  employees: LedgerRecord[]
  subcontractor_workers: LedgerRecord[]
  items: LedgerRecord[]
  slots: ScheduleSlot[]
  schedules: WorkTrackingRecord[]
}
export const workTrackingApi = {
  dailyReports: (month: string) => request<{ items: WorkTrackingRecord[] }>(`/daily-reports?month=${encodeURIComponent(month)}`),
  myDailyReports: (month: string) => request<{ items: WorkTrackingRecord[] }>(`/daily-reports/my?month=${encodeURIComponent(month)}`),
  patchDailyReport: (id: number, data: Record<string, unknown>) => request<{ item: WorkTrackingRecord }>(`/daily-reports/${id}`, { method: 'PATCH', body: JSON.stringify(data) }),
  deleteDailyReport: (id: number) => request<void>(`/daily-reports/${id}`, { method: 'DELETE' }),
  addDailyChild: (id: number) => request<{ item: WorkTrackingRecord }>(`/daily-reports/${id}/children`, { method: 'POST' }),
  duplicateDailyParent: (id: number) => request<{ item: WorkTrackingRecord }>(`/daily-reports/${id}/duplicate`, { method: 'POST' }),
  schedules: (month: string) => request<ScheduleResponse>(`/schedules?month=${encodeURIComponent(month)}`),
  createScheduleSlot: () => request<{ item: ScheduleSlot }>('/schedule-slots', { method: 'POST' }),
  patchScheduleCell: (data: Record<string, unknown>) => request<{ item: WorkTrackingRecord }>('/schedules/cell', { method: 'PATCH', body: JSON.stringify(data) }),
}

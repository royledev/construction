import type { AccountWorkerChoices, ClientSelector, EmployeeSelector, ProjectSelector, SelectorChoices, SubcontractorWorkerSelector } from './api'

export type ProjectMemberType = 'employee' | 'subcontractor'
export type ProjectMember = Record<string, unknown>
export type ProjectMemberRow = { key: string; index: number; member: ProjectMember }
export type ProjectMemberComposer = { memberType: ProjectMemberType; workerID: string }
export type ProjectMemberKeyFactory = (members: ProjectMember[]) => string
export type RelationshipKind = 'qualification' | 'worker'

const generatedCodeKeys: Record<string, string> = {
  projects: 'project_code',
  estimates: 'estimate_code',
  invoices: 'invoice_code',
  employees: 'employee_code',
  clients: 'client_code',
  subcontractors: 'subcontractor_code',
  'master-items': 'item_code',
}

export function generatedCodeKey(resource: string): string | undefined { return generatedCodeKeys[resource] }

export function masterSavePayload(resource: string, value: Record<string, unknown>): Record<string, unknown> {
  const payload = { ...value }
  delete payload.id
  const codeKey = generatedCodeKey(resource)
  if (codeKey) delete payload[codeKey]
  if (resource === 'employees') {
    for (const key of ['employment_insurance_no', 'pension_no', 'insurance_card_no', 'my_number_encrypted']) {
      if (payload[key] === '••••••') delete payload[key]
    }
  }
  if (resource === 'projects' && Array.isArray(payload.members)) payload.members = projectMembersPayload(payload.members)
  if (resource === 'employees' && Array.isArray(payload.qualifications)) payload.qualifications = relationshipRowsPayload(payload.qualifications)
  if (resource === 'subcontractors' && Array.isArray(payload.workers)) payload.workers = relationshipRowsPayload(payload.workers)
  if (resource !== 'accounts') return payload

  const workerType = payload.worker_type === 'subcontractor' ? 'subcontractor' : 'employee'
  const workerID = payload.worker_id ?? (workerType === 'employee' ? payload.employee_id : payload.subcontractor_worker_id)
  const accountPayload: Record<string, unknown> = {
    login_id: payload.login_id,
    password: payload.password,
    role: payload.role,
    worker_type: workerType,
    worker_id: workerID,
  }
  if (!accountPayload.password) delete accountPayload.password
  return accountPayload
}

export function clientOptionLabel(client: ClientSelector): string { return `${client.client_code} / ${client.name}` }
export function employeeOptionLabel(employee: EmployeeSelector): string { return `${employee.employee_code} / ${employee.full_name}` }
export function projectOptionLabel(project: ProjectSelector): string { return `${project.project_code} / ${project.site_name}` }
export function workerOptionLabel(worker: SubcontractorWorkerSelector): string { return `${worker.subcontractor_code} / ${worker.subcontractor_name} / ${worker.full_name}` }

export function workersForType(choices: AccountWorkerChoices, workerType: string): Array<EmployeeSelector | SubcontractorWorkerSelector> {
  return workerType === 'subcontractor' ? choices.subcontractor_workers : choices.employees
}

export function projectMemberType(member: ProjectMember): ProjectMemberType {
  return member.subcontractor_worker_id !== undefined && member.subcontractor_worker_id !== null && member.subcontractor_worker_id !== '' ? 'subcontractor' : 'employee'
}

export function projectMemberChoices(choices: SelectorChoices, memberType: ProjectMemberType): Array<EmployeeSelector | SubcontractorWorkerSelector> {
  return memberType === 'subcontractor' ? choices.subcontractor_workers : choices.employees
}

export function addProjectMember(members: ProjectMember[], memberType: ProjectMemberType, workerID: unknown, keyFactory: ProjectMemberKeyFactory = nextProjectMemberKey): ProjectMember[] {
  return [...members, { ...projectMemberSelection(memberType, workerID), _client_key: keyFactory(members) }]
}

export function projectMemberSelection(memberType: ProjectMemberType, workerID: unknown, member: ProjectMember = {}): ProjectMember {
  const next = { ...member }
  delete next.employee_id
  delete next.subcontractor_worker_id
  next[memberType === 'employee' ? 'employee_id' : 'subcontractor_worker_id'] = workerID
  return next
}

export function projectMemberRows(members: ProjectMember[], memberType: ProjectMemberType): ProjectMemberRow[] {
  return members.reduce<ProjectMemberRow[]>((rows, member, index) => {
    if (projectMemberType(member) === memberType) rows.push({ key: projectMemberRowKey(member), index, member })
    return rows
  }, [])
}

export function projectMemberRowKey(member: ProjectMember): string {
  const id = Number(member.id)
  if (Number.isInteger(id) && id > 0) return `member-${id}`
  return String(member._client_key ?? 'member-unsaved')
}

export function updateProjectMember(members: ProjectMember[], index: number, next: ProjectMember): ProjectMember[] {
  return members.map((member, memberIndex) => memberIndex === index ? next : member)
}

export function removeProjectMember(members: ProjectMember[], index: number): ProjectMember[] {
  return members.filter((_, memberIndex) => memberIndex !== index)
}

export function changeProjectMemberType(memberType: ProjectMemberType): ProjectMemberComposer {
  return { memberType, workerID: '' }
}

function nextProjectMemberKey(members: ProjectMember[]): string {
  const largest = members.reduce((max, member) => {
    const match = /^member-new-(\d+)$/.exec(String(member._client_key ?? ''))
    return match ? Math.max(max, Number(match[1])) : max
  }, 0)
  return `member-new-${largest + 1}`
}

export function projectMembersPayload(members: unknown[]): ProjectMember[] {
  return members.filter((member): member is ProjectMember => member !== null && typeof member === 'object').map(member => {
    const memberType = projectMemberType(member)
    const selectedID = memberType === 'employee' ? member.employee_id : member.subcontractor_worker_id
    const payload: ProjectMember = {}
    for (const key of ['work_description', 'start_on', 'end_on', 'phone', 'notes']) {
      if (key in member) payload[key] = member[key]
    }
    payload[memberType === 'employee' ? 'employee_id' : 'subcontractor_worker_id'] = selectedID
    return payload
  })
}

export function relationshipRowKey(row: ProjectMember, kind: RelationshipKind): string {
  const id = Number(row.id)
  if (Number.isInteger(id) && id > 0) return `${kind}-${id}`
  return String(row._client_key ?? `${kind}-unsaved`)
}

export function addRelationshipRow(rows: ProjectMember[], kind: RelationshipKind): ProjectMember[] {
  const largest = rows.reduce((max, row) => {
    const match = new RegExp(`^${kind}-new-(\\d+)$`).exec(String(row._client_key ?? ''))
    return match ? Math.max(max, Number(match[1])) : max
  }, 0)
  return [...rows, { id: 0, _client_key: `${kind}-new-${largest + 1}` }]
}

export function relationshipRowsPayload(rows: unknown[]): ProjectMember[] {
  return rows.filter((row): row is ProjectMember => row !== null && typeof row === 'object').map(row => {
    const { id, _client_key: ignoredClientKey, ...payload } = row
    void ignoredClientKey
    const persistedID = Number(id)
    if (Number.isInteger(persistedID) && persistedID > 0) payload.id = persistedID
    return payload
  })
}

export function isCurrentOrActiveChoice(choice: { id: number; status?: string; subcontractor_status?: string }, currentID: unknown): boolean {
  const status = choice.status ?? choice.subcontractor_status
  return choice.id === Number(currentID) || status === 'active'
}

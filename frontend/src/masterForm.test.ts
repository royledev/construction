import { describe, expect, it } from 'vitest'
import { addProjectMember, addRelationshipRow, changeProjectMemberType, clientOptionLabel, isCurrentOrActiveChoice, masterSavePayload, projectMemberChoices, projectMemberRows, projectMemberSelection, projectOptionLabel, relationshipRowKey, removeProjectMember, workerOptionLabel, workersForType } from './masterForm'

describe('master form payloads and selectors', () => {
  it('omits generated codes while keeping existing related-row ids', () => {
    expect(masterSavePayload('employees', {
      id: 8, employee_code: 'EMP-123456', full_name: '山田 太郎',
      qualifications: [{ id: 15, qualification_name: '玉掛け' }],
    })).toEqual({ full_name: '山田 太郎', qualifications: [{ id: 15, qualification_name: '玉掛け' }] })
    for (const [resource, codeKey] of Object.entries({ projects: 'project_code', clients: 'client_code', subcontractors: 'subcontractor_code', 'master-items': 'item_code' })) {
      expect(masterSavePayload(resource, { id: 9, [codeKey]: 'AUTO-123456', name: '入力値' })).toEqual({ name: '入力値' })
    }
  })

  it('converts account selection to the worker_type/worker_id API contract', () => {
    expect(masterSavePayload('accounts', { id: 3, login_id: 'taro', password: '', role: 'user', worker_type: 'subcontractor', subcontractor_worker_id: 12 })).toEqual({ login_id: 'taro', role: 'user', worker_type: 'subcontractor', worker_id: 12 })
  })

  it('renders selector choices with a public code and readable name', () => {
    expect(clientOptionLabel({ id: 1, client_code: 'CLI-123456', name: '建設商事', status: 'active' })).toBe('CLI-123456 / 建設商事')
    expect(projectOptionLabel({ id: 2, project_code: 'PRJ-123456', site_name: '新宿現場' })).toBe('PRJ-123456 / 新宿現場')
    expect(workerOptionLabel({ id: 3, subcontractor_code: 'SUB-123456', subcontractor_name: '協力工業', full_name: '佐藤 花子', subcontractor_status: 'active' })).toBe('SUB-123456 / 協力工業 / 佐藤 花子')
    const choices = { employees: [{ id: 4, employee_code: 'EMP-123456', full_name: '山田 太郎', status: 'active' as const }], subcontractor_workers: [] }
    expect(workersForType(choices, 'employee')).toEqual(choices.employees)
  })

  it('keeps an inactive existing relation visible but disables it for a new selection', () => {
    const retired = { id: 7, employee_code: 'EMP-000007', full_name: '退職 太郎', status: 'retired' }
    expect(isCurrentOrActiveChoice(retired, 7)).toBe(true)
    expect(isCurrentOrActiveChoice(retired, 8)).toBe(false)
  })

  it('links project member choices to the selected worker type and keeps inactive current members visible', () => {
    const choices = {
      clients: [], projects: [],
      employees: [{ id: 1, employee_code: 'EMP-000001', full_name: '自社 太郎', status: 'active' as const }, { id: 2, employee_code: 'EMP-000002', full_name: '退職 花子', status: 'retired' as const }],
      subcontractor_workers: [{ id: 3, subcontractor_code: 'SUB-000001', subcontractor_name: '協力工業', full_name: '外注 次郎', subcontractor_status: 'active' as const }],
    }
    expect(projectMemberChoices(choices, 'employee')).toEqual(choices.employees)
    expect(projectMemberChoices(choices, 'subcontractor')).toEqual(choices.subcontractor_workers)
    expect(isCurrentOrActiveChoice(choices.employees[1], 2)).toBe(true)
    expect(isCurrentOrActiveChoice(choices.employees[1], '')).toBe(false)
  })

  it('adds stable client-only keys and classifies project members into two row models', () => {
    const members = addProjectMember(addProjectMember([], 'employee', '4', () => 'member-new-1'), 'subcontractor', '9', () => 'member-new-2')
    expect(projectMemberRows(members, 'employee')).toEqual([{ index: 0, key: 'member-new-1', member: { employee_id: '4', _client_key: 'member-new-1' } }])
    expect(projectMemberRows(members, 'subcontractor')).toEqual([{ index: 1, key: 'member-new-2', member: { subcontractor_worker_id: '9', _client_key: 'member-new-2' } }])
    expect(projectMemberRows(removeProjectMember(members, 0), 'subcontractor')[0].key).toBe('member-new-2')
    expect(projectMemberRows([{ id: 44, _client_key: 'member-new-9', employee_id: '4' }], 'employee')[0].key).toBe('member-44')
  })

  it('resets the selected worker when the composer worker type changes', () => {
    expect(changeProjectMemberType('subcontractor')).toEqual({ memberType: 'subcontractor', workerID: '' })
  })

  it('makes project member IDs mutually exclusive on selection and in the API payload', () => {
    const switched = projectMemberSelection('subcontractor', '12', { id: 8, employee_id: '4', subcontractor_worker_id: 'old', employee_name: '残さない', work_description: '施工' })
    expect(switched).toEqual({ id: 8, employee_name: '残さない', work_description: '施工', subcontractor_worker_id: '12' })
    expect(masterSavePayload('projects', { id: 15, project_code: 'PRJ-000015', site_name: '現場', members: [switched, { employee_id: '6', notes: '自社として送る' }] })).toEqual({
      site_name: '現場',
      members: [{ subcontractor_worker_id: '12', work_description: '施工' }, { employee_id: '6', notes: '自社として送る' }],
    })
  })

  it('keeps nested qualification and worker keys stable while stripping client keys from API payloads', () => {
    const qualifications = addRelationshipRow(addRelationshipRow([], 'qualification'), 'qualification')
    expect(relationshipRowKey(qualifications.slice(1)[0], 'qualification')).toBe('qualification-new-2')
    expect(relationshipRowKey({ id: 31, _client_key: 'qualification-new-9' }, 'qualification')).toBe('qualification-31')
    expect(masterSavePayload('employees', { id: 8, full_name: '山田 太郎', qualifications: [{ id: 15, _client_key: 'qualification-new-9', qualification_name: '玉掛け' }, ...qualifications] })).toEqual({
      full_name: '山田 太郎',
      qualifications: [{ id: 15, qualification_name: '玉掛け' }, {}, {}],
    })
    const workers = addRelationshipRow([], 'worker')
    expect(masterSavePayload('subcontractors', { id: 9, name: '協力工業', workers: [{ id: 22, _client_key: 'worker-new-9', full_name: '外注 太郎' }, ...workers] })).toEqual({
      name: '協力工業', workers: [{ id: 22, full_name: '外注 太郎' }, {}],
    })
  })
})

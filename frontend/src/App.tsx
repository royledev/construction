import { useEffect, useId, useState, type FormEvent, type ReactNode } from 'react'
import { AccountTreeOutlined, Add, AssessmentOutlined, BadgeOutlined, BusinessOutlined, CalendarMonthOutlined, ChevronRight, Close, DeleteOutline, DescriptionOutlined, EditOutlined, FolderOpenOutlined, Logout, Menu, PersonOutline, PrintOutlined, ReceiptLongOutlined, SaveOutlined, SettingsOutlined, TableChartOutlined, WorkOutline } from '@mui/icons-material'
import { Alert, Avatar, Box, Button, CircularProgress, CssBaseline, Dialog, DialogActions, DialogContent, DialogTitle, Divider, Drawer, FormControl, IconButton, InputLabel, List, ListItemButton, ListItemIcon, ListItemText, MenuItem, Paper, Select, Stack, Table, TableBody, TableCell, TableContainer, TableHead, TableRow, TextField, Tooltip, Typography, useMediaQuery, useTheme } from '@mui/material'
import { authApi, documentApi, ledgerApi, selectorApi, type AccountWorkerChoices, type LedgerRecord, type SelectorChoices, type User } from './api'
import { addDocumentLine, choicesWithCurrent, documentLineRowKey, documentSavePayload, documentTypeLabel, documentTypeOptions, normalizeDocumentForm, validateDocumentForm, withDocumentLineClientKeys, type DocumentResource } from './documentForm'
import { calculateDocumentAmounts, documentYen, MAX_DOCUMENT_DECIMAL_CENTS } from './documentAmounts'
import { shouldShrinkFieldLabel } from './fieldLabel'
import { displayLedgerValue } from './ledgerDisplay'
import { loginConfig } from './loginConfig'
import { addProjectMember, addRelationshipRow, changeProjectMemberType, clientOptionLabel, employeeOptionLabel, generatedCodeKey, isCurrentOrActiveChoice, masterSavePayload, projectMemberChoices, projectMemberRows, projectMemberSelection, projectOptionLabel, relationshipRowKey, removeProjectMember, updateProjectMember, workerOptionLabel, workersForType, type ProjectMemberType, type RelationshipKind } from './masterForm'
import { WorkTrackingWorkspace } from './WorkTrackingWorkspace'

type MenuEntry = { label: string; icon: ReactNode; group?: string }
const menu: MenuEntry[] = [
  { label: '案件', icon: <FolderOpenOutlined /> }, { label: '見積', icon: <DescriptionOutlined /> }, { label: '請求', icon: <ReceiptLongOutlined /> },
  { label: '従業員', icon: <BadgeOutlined /> }, { label: '得意先', icon: <BusinessOutlined /> }, { label: '外注先', icon: <WorkOutline /> },
  { label: '日報', icon: <AssessmentOutlined /> }, { label: '工程表', icon: <CalendarMonthOutlined /> },
  { label: '作業項目', icon: <TableChartOutlined />, group: '設定' }, { label: '見積・請求項目', icon: <TableChartOutlined />, group: '設定' }, { label: '単価項目', icon: <TableChartOutlined />, group: '設定' }, { label: '工程項目', icon: <TableChartOutlined />, group: '設定' }, { label: 'アカウント', icon: <PersonOutline />, group: '設定' },
]

const drawerWidth = 272

export function App() {
  const [user, setUser] = useState<User | null>(null); const [loading, setLoading] = useState(true)
  useEffect(() => { authApi.me().then(({ user: current }) => setUser(current)).catch(() => undefined).finally(() => setLoading(false)) }, [])
  if (loading) return <Box className="loading-screen"><CircularProgress color="secondary" /><Typography>台帳を開いています</Typography></Box>
  return user ? <ApplicationShell user={user} onLogout={() => { authApi.logout().finally(() => setUser(null)) }} /> : <LoginScreen onLogin={setUser} />
}

function LoginScreen({ onLogin }: { onLogin: (user: User) => void }) {
  const [loginId, setLoginId] = useState(loginConfig.loginId); const [password, setPassword] = useState(loginConfig.password); const [error, setError] = useState(''); const [submitting, setSubmitting] = useState(false)
  const submit = async (event: FormEvent) => { event.preventDefault(); setSubmitting(true); setError(''); try { const result = await authApi.login(loginId, password); onLogin(result.user) } catch (err) { setError(err instanceof Error ? err.message : 'ログインできませんでした。') } finally { setSubmitting(false) } }
  return <Box className="login-page"><Box className="login-blueprint" aria-hidden="true"><span>PROJECT LEDGER</span><b>01</b><i /><i /><i /></Box><Paper component="form" onSubmit={submit} className="login-card"><Stack spacing={3}><Box><Typography className="eyebrow">CONSTRUCTION / 現場工事案件管理</Typography><Typography variant="h2">現場の予定と記録を、<br />ひとつの台帳に。</Typography></Box><Typography color="text.secondary">担当する工事の状況を確認するにはログインしてください。</Typography>{error && <Alert severity="error">{error}</Alert>}<TextField label="ログインID" autoComplete="username" value={loginId} onChange={(event) => setLoginId(event.target.value)} /><TextField label="パスワード" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} /><Button type="submit" variant="contained" color="secondary" disabled={submitting}>{submitting ? 'ログイン中…' : 'ログインする'}<ChevronRight /></Button>{loginConfig.showDemoCredentials && <Typography variant="caption" color="text.secondary">デモ: admin / construction-demo</Typography>}</Stack></Paper></Box>
}

function ApplicationShell({ user, onLogout }: { user: User; onLogout: () => void }) {
  const theme = useTheme(); const desktop = useMediaQuery(theme.breakpoints.up('md')); const [open, setOpen] = useState(false); const [selected, setSelected] = useState('案件')
  const navigation = <Navigation selected={selected} onSelect={(label) => { setSelected(label); setOpen(false) }} user={user} onLogout={onLogout} />
  return <Box sx={{ display: 'flex', minHeight: '100dvh' }}><CssBaseline />
    {!desktop && <Tooltip title={open ? 'ナビゲーションを閉じる' : 'ナビゲーションを開く'}><IconButton className="mobile-nav-trigger" aria-label={open ? 'ナビゲーションを閉じる' : 'ナビゲーションを開く'} aria-expanded={open} aria-controls="mobile-navigation" onClick={() => setOpen(value => !value)}><Menu /></IconButton></Tooltip>}
    <Box component="nav" id="mobile-navigation">{desktop ? <Drawer variant="permanent" open PaperProps={{ sx: { width: drawerWidth, border: 0 } }}>{navigation}</Drawer> : <Drawer variant="temporary" open={open} onClose={() => setOpen(false)} ModalProps={{ keepMounted: true }} PaperProps={{ sx: { width: drawerWidth, border: 0 } }}>{navigation}</Drawer>}</Box>
    <Box component="main" className="main-area" sx={{ width: { md: `calc(100% - ${drawerWidth}px)` }, ml: { md: `${drawerWidth}px` } }}><Workspace title={selected} user={user} /></Box>
  </Box>
}

function Navigation({ selected, onSelect, user, onLogout }: { selected: string; onSelect: (label: string) => void; user: User; onLogout: () => void }) {
  let settingsShown = false
  return <Box className="drawer-content"><Box className="brand-block"><AccountTreeOutlined /><Box><Typography className="brand-name">CONSTRUCTION</Typography><Typography className="brand-sub">PROJECT LEDGER / 2026</Typography></Box></Box><Divider />
    <List className="navigation-list">{menu.map((entry) => <Box key={entry.label}>{entry.group && !settingsShown && (settingsShown = true, <Typography className="nav-section"><SettingsOutlined fontSize="inherit" /> {entry.group}</Typography>)}<ListItemButton selected={selected === entry.label} onClick={() => onSelect(entry.label)} className="nav-item"><ListItemIcon>{entry.icon}</ListItemIcon><ListItemText primary={entry.label} /></ListItemButton></Box>)}</List>
    <Box className="account-plate"><Divider /><Stack direction="row" alignItems="center" spacing={1.25}><Avatar>{user.displayName.slice(0, 1)}</Avatar><Box flex={1} minWidth={0}><Typography noWrap fontWeight={700}>{user.displayName}</Typography><Typography variant="caption" color="text.secondary">{user.role === 'admin' ? '管理者' : '一般利用者'}</Typography></Box><IconButton aria-label="ログアウト" onClick={onLogout} size="small"><Logout fontSize="small" /></IconButton></Stack></Box>
  </Box>
}

type Field = { key: string; label: string; type?: 'date' | 'select' | 'textarea'; choices?: [string, string][] }
type Ledger = { resource: string; query?: string; columns: Field[]; fields: Field[]; initial: LedgerRecord; relationship?: 'employee' | 'worker' | 'member' }
const employeeStatusChoices: [string, string][] = [['active', '在籍'], ['retired', '退職']]
const partnerStatusChoices: [string, string][] = [['active', '有効'], ['inactive', '無効']]
const ledgerFor = (title: string): Ledger | null => {
  const settings: Record<string, string> = { '作業項目': 'work', '見積・請求項目': 'estimate_invoice', '単価項目': 'unit_price', '工程項目': 'schedule' }
  if (settings[title]) return { resource: 'master-items', query: `?type=${settings[title]}`, columns: [{ key: 'item_code', label: '項目コード' }, { key: 'name', label: '項目名' }, { key: 'notes', label: '備考' }], fields: [{ key: 'name', label: '項目名' }, { key: 'notes', label: '備考', type: 'textarea' }], initial: { id: 0, item_type: settings[title], name: '', notes: '' } }
  if (title === 'アカウント') return { resource: 'accounts', columns: [{ key: 'login_id', label: 'ログインID' }, { key: 'worker_type', label: '作業者区分' }, { key: 'worker_name', label: '作業者名' }, { key: 'role', label: '権限' }], fields: [], initial: { id: 0, login_id: '', worker_type: 'employee', worker_id: '', password: '', role: 'user' } }
  const ledgers: Record<string, Ledger> = {
    '案件': { resource: 'projects', columns: [{ key: 'project_code', label: '案件コード' }, { key: 'site_name', label: '現場名' }, { key: 'client_name', label: '得意先名' }, { key: 'ordered_on', label: '受注日' }, { key: 'construction_start_on', label: '工期開始' }, { key: 'construction_end_on', label: '工期終了' }, { key: 'status', label: '進捗' }], fields: [{ key: 'site_name', label: '現場名' }, { key: 'site_address', label: '現場住所' }, { key: 'client_id', label: '得意先' }, { key: 'manager_employee_id', label: '現場担当作業員' }, { key: 'ordered_on', label: '受注日', type: 'date' }, { key: 'construction_start_on', label: '工期開始日', type: 'date' }, { key: 'construction_end_on', label: '工期終了日', type: 'date' }, { key: 'completed_on', label: '工事完了日', type: 'date' }, { key: 'lost_on', label: '失注日', type: 'date' }, { key: 'status', label: '進捗', type: 'select', choices: [['unreceived', '未受注'], ['ordered', '受注'], ['completed', '完了'], ['lost', '失注']] }, { key: 'notes', label: '備考', type: 'textarea' }], initial: { id: 0, site_name: '', client_id: '', status: 'unreceived', members: [] }, relationship: 'member' },
    '従業員': { resource: 'employees', columns: [{ key: 'employee_code', label: '従業員コード' }, { key: 'status', label: '状態' }, { key: 'full_name', label: '従業員名' }, { key: 'occupation', label: '職種' }], fields: [{ key: 'status', label: '状態', type: 'select', choices: employeeStatusChoices }, { key: 'full_name', label: '氏名' }, { key: 'full_name_kana', label: 'ふりがな' }, { key: 'occupation', label: '職種' }, { key: 'birth_date', label: '生年月日', type: 'date' }, { key: 'hired_on', label: '入社日', type: 'date' }, { key: 'retired_on', label: '退社日', type: 'date' }, { key: 'postal_code', label: '郵便番号' }, { key: 'address', label: '住所' }, { key: 'phone', label: '電話番号' }, { key: 'employment_insurance_no', label: '雇用保険番号（マスク表示）' }, { key: 'pension_no', label: '基礎年金番号（マスク表示）' }, { key: 'insurance_card_no', label: '保険証番号（マスク表示）' }, { key: 'my_number_encrypted', label: 'マイナンバー（マスク表示）' }], initial: { id: 0, status: 'active', full_name: '', qualifications: [], workItemIds: [] }, relationship: 'employee' },
    '得意先': { resource: 'clients', columns: [{ key: 'client_code', label: '得意先コード' }, { key: 'status', label: '状態' }, { key: 'name', label: '得意先名' }, { key: 'contact_name', label: '担当者' }, { key: 'contact_phone', label: '担当者電話番号' }], fields: [{ key: 'status', label: '状態', type: 'select', choices: partnerStatusChoices }, { key: 'name', label: '得意先名' }, { key: 'honorific', label: '呼称', type: 'select', choices: [['御中', '御中'], ['様', '様']] }, { key: 'kana', label: 'ふりがな' }, { key: 'phone', label: '電話番号' }, { key: 'fax', label: 'FAX番号' }, { key: 'email', label: 'メール' }, { key: 'postal_code', label: '郵便番号' }, { key: 'address', label: '住所' }, { key: 'contact_name', label: '担当者名' }, { key: 'contact_phone', label: '担当者電話番号' }, { key: 'registration_number', label: '登録番号' }, { key: 'closing_day', label: '請求締め日（1-31）' }, { key: 'payment_due_rule', label: '入金予定' }, { key: 'payment_method', label: '支払条件' }, { key: 'payment_site_days', label: 'サイト（日）' }, { key: 'bank_name', label: '銀行名' }, { key: 'bank_branch', label: '支店名' }, { key: 'bank_account_no', label: '口座番号' }, { key: 'notes', label: '備考', type: 'textarea' }], initial: { id: 0, status: 'active', name: '', honorific: '御中' } },
    '外注先': { resource: 'subcontractors', columns: [{ key: 'subcontractor_code', label: '外注先コード' }, { key: 'status', label: '状態' }, { key: 'name', label: '外注先名' }, { key: 'contact_name', label: '担当者' }, { key: 'contact_phone', label: '担当者電話番号' }], fields: [{ key: 'status', label: '状態', type: 'select', choices: partnerStatusChoices }, { key: 'name', label: '外注先名' }, { key: 'honorific', label: '呼称', type: 'select', choices: [['御中', '御中'], ['様', '様']] }, { key: 'kana', label: 'ふりがな' }, { key: 'phone', label: '電話番号' }, { key: 'fax', label: 'FAX番号' }, { key: 'email', label: 'メール' }, { key: 'postal_code', label: '郵便番号' }, { key: 'address', label: '住所' }, { key: 'contact_name', label: '担当者名' }, { key: 'contact_phone', label: '担当者電話番号' }, { key: 'registration_number', label: '登録番号' }, { key: 'payment_method', label: '支払条件' }, { key: 'payment_site_days', label: 'サイト（日）' }, { key: 'bank_name', label: '銀行名' }, { key: 'bank_branch', label: '支店名' }, { key: 'bank_account_no', label: '口座番号' }, { key: 'notes', label: '備考', type: 'textarea' }], initial: { id: 0, status: 'active', name: '', honorific: '御中', workers: [] }, relationship: 'worker' },
  }
  return ledgers[title] ?? null
}

type DocumentLine = { id?: number; _client_key?: string; item_name: string; quantity: string; unit: string; unit_price: string; notes: string }
const emptyLine = (): DocumentLine => ({ item_name: '', quantity: '1', unit: '式', unit_price: '0', notes: '' })
const todayISO = () => new Date().toLocaleDateString('en-CA', { timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone })
const emptyDocument = (resource: 'estimates' | 'invoices'): LedgerRecord => resource === 'estimates'
  ? { id: 0, project_id: '', issued_on: todayISO(), status: 'draft', discount_amount: '0', lines: [emptyLine()] }
  : { id: 0, project_id: '', issued_on: todayISO(), billing_type: 'individual', discount_amount: '0', lines: [emptyLine()] }

export function DocumentWorkspace({ title, canEdit }: { title: '見積' | '請求'; canEdit: boolean }) {
  const resource = title === '見積' ? 'estimates' : 'invoices'; const [items, setItems] = useState<LedgerRecord[]>([]); const [error, setError] = useState(''); const [loading, setLoading] = useState(true); const [editing, setEditing] = useState<LedgerRecord | null>(null); const [deleting, setDeleting] = useState<LedgerRecord | null>(null)
  const reload = () => { setLoading(true); ledgerApi.list(resource).then(({ items: next }) => setItems(Array.isArray(next) ? next : [])).catch((err: Error) => setError(err.message)).finally(() => setLoading(false)) }
  useEffect(reload, [resource])
  const remove = async () => { if (!deleting) return; try { await ledgerApi.remove(resource, deleting.id); setDeleting(null); reload() } catch (err) { setError(err instanceof Error ? err.message : '削除できませんでした。') } }
  const codeKey = resource === 'estimates' ? 'estimate_code' : 'invoice_code'
  return <Box><Box className="workspace-header"><Box><Typography className="eyebrow">台帳 / {title}</Typography><Typography variant="h1">{title}</Typography></Box><Box className="date-stamp"><b>25</b><span>JUL<br />2026</span></Box></Box><Paper className="workspace-paper"><Box className="paper-ruler"><span>COMMERCIAL DOCUMENTS</span><span>04 / {title}</span></Box><Box sx={{ p: { xs: 1.5, sm: 3 } }}>{error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}<Box className="ledger-toolbar"><Typography color="text.secondary">案件と得意先に紐づく{title}を管理します。</Typography>{canEdit && <Tooltip title="追加"><IconButton className="action-add" aria-label={`${title}を追加`} onClick={() => setEditing(emptyDocument(resource))}><Add /></IconButton></Tooltip>}</Box><TableContainer className="ledger-table"><Table size="small"><TableHead><TableRow><TableCell>{title}コード</TableCell><TableCell>発行日</TableCell><TableCell>案件コード</TableCell><TableCell>現場名</TableCell><TableCell>得意先名</TableCell><TableCell align="right">{title}金額</TableCell><TableCell>{title === '見積' ? 'ステータス' : '請求区分'}</TableCell><TableCell className="action-column" component="th" scope="col">操作</TableCell></TableRow></TableHead><TableBody>{loading ? <TableRow><TableCell colSpan={8} align="center"><CircularProgress size={23} /></TableCell></TableRow> : items.length === 0 ? <TableRow><TableCell colSpan={8} align="center">データがありません。</TableCell></TableRow> : items.map((item) => <TableRow hover key={item.id}><TableCell>{display(item[codeKey])}</TableCell><TableCell>{display(item.issued_on)}</TableCell><TableCell>{display(item.project_code)}</TableCell><TableCell>{display(item.site_name)}</TableCell><TableCell>{display(item.client_name)}</TableCell><TableCell align="right" className="money-cell">{currency(item.total)}</TableCell><TableCell>{documentTypeLabel(resource, item.document_type)}</TableCell><TableCell className="action-column"><Tooltip title="詳細"><IconButton aria-label="詳細" size="small" onClick={() => setEditing(item)}><EditOutlined fontSize="small" /></IconButton></Tooltip>{canEdit && <Tooltip title="削除"><IconButton aria-label="削除" size="small" color="error" onClick={() => setDeleting(item)}><DeleteOutline fontSize="small" /></IconButton></Tooltip>}</TableCell></TableRow>)}</TableBody></Table></TableContainer></Box></Paper>{editing && <DocumentDialog title={title} resource={resource} initial={editing} canEdit={canEdit} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); reload() }} />}{deleting && <Dialog open onClose={() => setDeleting(null)}><DialogTitle>論理削除の確認</DialogTitle><DialogContent>「{display(deleting[codeKey])}」を削除します。履歴は保持されます。</DialogContent><DialogActions><Button onClick={() => setDeleting(null)}>キャンセル</Button><Button color="error" variant="contained" onClick={remove}>削除する</Button></DialogActions></Dialog>}<Typography className="copyright">© 2026 CONSTRUCTION · PROJECT MANAGEMENT LEDGER</Typography></Box>
}

export type DocumentDialogDataSource = {
  loadSelectors: () => Promise<SelectorChoices>
  loadDocument: (resource: DocumentResource, id: number) => Promise<{ item: LedgerRecord }>
  create: (resource: DocumentResource, payload: LedgerRecord) => Promise<unknown>
  update: (resource: DocumentResource, id: number, payload: LedgerRecord) => Promise<unknown>
}

const defaultDocumentDialogDataSource: DocumentDialogDataSource = {
  loadSelectors: selectorApi.list,
  loadDocument: ledgerApi.get,
  create: ledgerApi.create,
  update: ledgerApi.update,
}

export function DocumentDialog({ title, resource, initial, canEdit: requestedCanEdit, onClose, onSaved, dataSource = defaultDocumentDialogDataSource }: { title: '見積' | '請求'; resource: DocumentResource; initial: LedgerRecord; canEdit: boolean; onClose: () => void; onSaved: () => void; dataSource?: DocumentDialogDataSource }) {
  const [value, setValue] = useState<LedgerRecord>(() => normalizeDocumentForm(resource, initial) as LedgerRecord)
  const [projects, setProjects] = useState<SelectorChoices['projects']>([])
  const [error, setError] = useState(''); const [saving, setSaving] = useState(false); const [dirty, setDirty] = useState(false); const isNew = initial.id === 0; const [detailReady, setDetailReady] = useState(isNew)
  const fieldPrefix = useId(); const projectLabelID = `${fieldPrefix}-project-label`; const projectSelectID = `${fieldPrefix}-project`; const typeLabelID = `${fieldPrefix}-type-label`; const typeSelectID = `${fieldPrefix}-type`
  const canEdit = requestedCanEdit && detailReady && !saving
  const codeKey = resource === 'estimates' ? 'estimate_code' : 'invoice_code'; const typeKey = resource === 'estimates' ? 'status' : 'billing_type'
  const projectChoices = choicesWithCurrent(projects, value.project_id, id => ({ id, project_code: `ID: ${id}`, site_name: '現在の案件' }))
  useEffect(() => { dataSource.loadSelectors().then(({ projects: next }) => setProjects(next)).catch((err: Error) => setError(err.message)) }, [dataSource])
  useEffect(() => {
    if (isNew) { setDetailReady(true); return }
    let current = true
    setDetailReady(false)
    dataSource.loadDocument(resource, initial.id).then(({ item }) => {
      if (!current) return
      setValue(normalizeDocumentForm(resource, item) as LedgerRecord)
      setDirty(false)
      setDetailReady(true)
    }).catch((err: Error) => { if (current) setError(err.message) })
    return () => { current = false }
  }, [dataSource, initial.id, isNew, resource])
  const changeValue = (next: (old: LedgerRecord) => LedgerRecord) => { if (!canEdit) return; setDirty(true); setValue(next) }
  const set = (key: string, next: string) => changeValue(old => ({ ...old, [key]: next }))
  const lines = asDocumentLines(value.lines); const calculation = calculateDocumentAmounts(lines, value.discount_amount ?? '0'); const validationError = validateDocumentForm(resource, { ...value, lines }); let payloadSizeError: string | null = null
  if (!validationError && calculation.ok) {
    try { documentSavePayload(resource, { ...value, lines }) } catch (err) { payloadSizeError = err instanceof Error ? err.message : '保存内容を確認してください。' }
  }
  const formError = validationError ?? payloadSizeError ?? (calculation.ok ? null : calculation.error)
  const updateLine = (index: number, key: keyof DocumentLine, next: string) => changeValue(old => ({ ...old, lines: lines.map((line, lineIndex) => lineIndex === index ? { ...line, [key]: next } : line) }))
  const save = async () => { if (!canEdit || formError) return; setSaving(true); setError(''); try { const payload = documentSavePayload(resource, { ...value, lines }) as LedgerRecord; if (isNew) await dataSource.create(resource, payload); else await dataSource.update(resource, value.id, payload); onSaved() } catch (err) { setError(err instanceof Error ? err.message : '保存できませんでした。') } finally { setSaving(false) } }
  const download = async () => { try { await documentApi.downloadPdf(resource, value.id) } catch (err) { setError(err instanceof Error ? err.message : 'PDFを生成できませんでした。') } }
  const pdfAvailable = !isNew && detailReady && !dirty && !saving
  const totals = calculation.ok ? calculation.value : null
  return <Dialog open fullWidth maxWidth="lg" onClose={saving ? () => undefined : onClose} aria-labelledby="document-dialog-title">
<DialogTitle id="document-dialog-title" className="document-dialog-title">{isNew ? `${title}を作成` : `${title}詳細`}<Box className="document-dialog-title-actions">{!isNew && <Tooltip title={pdfAvailable ? 'PDFをダウンロード' : '保存後にPDFへ反映'}>
<span>
<IconButton aria-label="PDFをダウンロード" disabled={!pdfAvailable} onClick={download}>
<PrintOutlined />
</IconButton>
</span>
</Tooltip>}<IconButton aria-label="閉じる" disabled={saving} onClick={onClose}>
<Close />
</IconButton>
</Box>
</DialogTitle>
<DialogContent dividers>{error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}<Box className="form-grid">{!isNew && <CodeField value={value[codeKey]} />}<FormControl fullWidth size="small">
<InputLabel id={projectLabelID}>案件</InputLabel>
<Select id={projectSelectID} labelId={projectLabelID} label="案件" value={String(value.project_id ?? '')} disabled={!canEdit} onChange={e => set('project_id', e.target.value)}>{projectChoices.map(({ choice: project, isFallback }) => <MenuItem key={project.id} value={project.id} disabled={isFallback}>{projectOptionLabel(project)}</MenuItem>)}</Select>
</FormControl>
<TextField fullWidth label="発行日" type="date" InputLabelProps={{ shrink: true }} value={String(value.issued_on ?? '')} disabled={!canEdit} onChange={e => set('issued_on', e.target.value)} />
<FormControl fullWidth size="small">
<InputLabel id={typeLabelID}>{title === '見積' ? 'ステータス' : '請求区分'}</InputLabel>
<Select id={typeSelectID} labelId={typeLabelID} label={title === '見積' ? 'ステータス' : '請求区分'} value={String(value[typeKey] ?? '')} disabled={!canEdit} onChange={e => set(typeKey, e.target.value)}>{documentTypeOptions(resource).map(([optionValue, optionLabel]) => <MenuItem key={optionValue} value={optionValue}>{optionLabel}</MenuItem>)}</Select>
</FormControl>
<TextField fullWidth label="請負者会社名" value={String(value.contractor_name ?? '')} disabled={!canEdit} onChange={e => set('contractor_name', e.target.value)} />
<TextField fullWidth label="代表者名" value={String(value.contractor_representative ?? '')} disabled={!canEdit} onChange={e => set('contractor_representative', e.target.value)} />
<TextField fullWidth label="請負者住所" value={String(value.contractor_address ?? '')} disabled={!canEdit} onChange={e => set('contractor_address', e.target.value)} />
<TextField fullWidth label="値引き（円）" type="number" inputProps={{ min: 0, max: MAX_DOCUMENT_DECIMAL_CENTS.toString().replace(/(\d{2})$/, '.$1'), step: .01 }} value={String(value.discount_amount ?? '0')} disabled={!canEdit} onChange={e => set('discount_amount', e.target.value)} />
</Box>{!isNew && <Box className="document-meta">
<span>案件: {display(value.project_code)} / {display(value.site_name)}</span>
<span>得意先: {display(value.client_name)} {display(value.client_honorific)}</span>
</Box>}<Box className="detail-section">
<Box className="document-lines-heading">
<Typography className="detail-heading">明細</Typography>{requestedCanEdit && <Button size="small" startIcon={<Add />} disabled={!canEdit} onClick={() => changeValue(old => ({ ...old, lines: addDocumentLine(lines, emptyLine()) }))}>明細を追加</Button>}</Box>{formError && <Alert severity="warning" sx={{ mt: 1 }}>{formError}{!calculation.ok && ' 金額は「—」として表示され、'} 保存できません。</Alert>}<TableContainer className="document-lines">
<Table size="small">
<TableHead>
<TableRow>
<TableCell className="document-item-column">項目名</TableCell>
<TableCell>数量</TableCell>
<TableCell>単位</TableCell>
<TableCell>単価</TableCell>
<TableCell align="right">金額</TableCell>
<TableCell>備考</TableCell>
<TableCell className="action-column" component="th" scope="col">操作</TableCell>
</TableRow>
</TableHead>
<TableBody>{lines.map((line, index) => <TableRow key={documentLineRowKey(line)}>
<TableCell className="document-item-column">
<TextField inputProps={{ 'aria-label': `明細${index + 1}項目名` }} variant="standard" value={line.item_name} disabled={!canEdit} onChange={e => updateLine(index, 'item_name', e.target.value)} />
</TableCell>
<TableCell>
<TextField variant="standard" type="number" inputProps={{ 'aria-label': `明細${index + 1}数量`, min: .001, max: '99999999999.999', step: .001 }} value={line.quantity} disabled={!canEdit} onChange={e => updateLine(index, 'quantity', e.target.value)} />
</TableCell>
<TableCell>
<TextField inputProps={{ 'aria-label': `明細${index + 1}単位` }} variant="standard" value={line.unit} disabled={!canEdit} onChange={e => updateLine(index, 'unit', e.target.value)} />
</TableCell>
<TableCell>
<TextField variant="standard" type="number" inputProps={{ 'aria-label': `明細${index + 1}単価`, min: 0, max: '999999999999.99', step: .01 }} value={line.unit_price} disabled={!canEdit} onChange={e => updateLine(index, 'unit_price', e.target.value)} />
</TableCell>
<TableCell align="right" className="money-cell">{totals ? documentYen(totals.amounts[index]) : '—'}</TableCell>
<TableCell>
<TextField inputProps={{ 'aria-label': `明細${index + 1}備考` }} variant="standard" value={line.notes} disabled={!canEdit} onChange={e => updateLine(index, 'notes', e.target.value)} />
</TableCell>
<TableCell className="action-column">{requestedCanEdit && <IconButton aria-label={`明細${index + 1}を削除`} color="error" size="small" disabled={!canEdit || lines.length === 1} onClick={() => changeValue(old => ({ ...old, lines: lines.filter((_, lineIndex) => lineIndex !== index) }))}>
<DeleteOutline fontSize="small" />
</IconButton>}</TableCell>
</TableRow>)}</TableBody>
</Table>
</TableContainer>
<Box className="document-totals">
<span>合計 {totals ? documentYen(totals.subtotal) : '—'}</span>
<span>値引き {totals ? documentYen(totals.discount) : '—'}</span>
<span>税抜き合計 {totals ? documentYen(totals.preTax) : '—'}</span>
<span>消費税（10%） {totals ? documentYen(totals.tax) : '—'}</span>
<b>{title === '見積' ? '御見積合計' : '御請求合計'} {totals ? documentYen(totals.total) : '—'}</b>
</Box>
</Box>{!pdfAvailable && !isNew && <Typography variant="caption" color="text.secondary">保存後にPDFへ反映されます。</Typography>}</DialogContent>
<DialogActions>
<Button disabled={saving} onClick={onClose}>閉じる</Button>{requestedCanEdit && <Button variant="contained" color="secondary" startIcon={<SaveOutlined />} disabled={!canEdit || formError !== null} onClick={save}>{saving ? '保存中…' : '保存'}</Button>}</DialogActions>
</Dialog>
}

function asDocumentLines(value: unknown): DocumentLine[] { const lines = Array.isArray(value) ? value.filter((line): line is Record<string, unknown> => line !== null && typeof line === 'object').map(line => ({ id: Number.isInteger(Number(line.id)) && Number(line.id) > 0 ? Number(line.id) : undefined, _client_key: typeof line._client_key === 'string' ? line._client_key : undefined, item_name: String(line.item_name ?? ''), quantity: String(line.quantity ?? 0), unit: String(line.unit ?? ''), unit_price: String(line.unit_price ?? 0), notes: String(line.notes ?? '') })) : [emptyLine()]; return withDocumentLineClientKeys(lines.length ? lines : [emptyLine()]) }
function currency(value: unknown) { const amount = Number(value ?? 0); return `¥${Number.isFinite(amount) ? Math.round(amount).toLocaleString('ja-JP') : '0'}` }

function Workspace({ title, user }: { title: string; user: User }) {
  if (title === '見積' || title === '請求') return <DocumentWorkspace title={title} canEdit={user.role === 'admin'} />
  if (title === '日報' || title === '工程表') return <WorkTrackingWorkspace title={title} user={user} />
  const ledger = ledgerFor(title)
  return <Box><Box className="workspace-header"><Box><Typography className="eyebrow">台帳 / {title}</Typography><Typography variant="h1">{title}</Typography></Box><Box className="date-stamp"><b>25</b><span>JUL<br />2026</span></Box></Box><Paper className="workspace-paper"><Box className="paper-ruler"><span>DRAWING INDEX</span><span>01 / {title}</span></Box>{ledger ? <LedgerTable title={title} ledger={ledger} canEdit={user.role === 'admin'} /> : <Box sx={{ p: { xs: 2.5, sm: 4 } }}><Typography variant="h5" fontWeight={800} gutterBottom>{title}の台帳</Typography><Typography color="text.secondary">この機能は次の作業単位で追加します。</Typography><Box className="empty-grid"><Box /><Box /><Box /></Box></Box>}</Paper><Typography className="copyright">© 2026 CONSTRUCTION · PROJECT MANAGEMENT LEDGER</Typography></Box>
}

export function LedgerTable({ title, ledger, canEdit }: { title: string; ledger: Ledger; canEdit: boolean }) {
  const [items, setItems] = useState<LedgerRecord[]>([]); const [loading, setLoading] = useState(true); const [error, setError] = useState(''); const [editing, setEditing] = useState<LedgerRecord | null>(null); const [deleting, setDeleting] = useState<LedgerRecord | null>(null)
  const reload = () => { setLoading(true); ledgerApi.list(ledger.resource, ledger.query).then(({ items: records }) => setItems(records)).catch((err: Error) => setError(err.message)).finally(() => setLoading(false)) }
  useEffect(reload, [ledger.resource, ledger.query])
  const remove = async () => { if (!deleting) return; try { await ledgerApi.remove(ledger.resource, deleting.id); setDeleting(null); reload() } catch (err) { setError(err instanceof Error ? err.message : '削除できませんでした。') } }
  return <Box sx={{ p: { xs: 1.5, sm: 3 } }}>{error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}<Box className="ledger-toolbar"><Typography color="text.secondary">{title}を一覧で確認・編集します。</Typography>{canEdit && <Tooltip title="追加"><IconButton className="action-add" aria-label={`${title}を追加`} onClick={() => setEditing(ledger.initial)}><Add /></IconButton></Tooltip>}</Box><TableContainer className="ledger-table"><Table size="small"><TableHead><TableRow>{ledger.columns.map((column) => <TableCell key={column.key}>{column.label}</TableCell>)}<TableCell className="action-column" component="th" scope="col">操作</TableCell></TableRow></TableHead><TableBody>{loading ? <TableRow><TableCell colSpan={ledger.columns.length + 1} align="center"><CircularProgress size={23} /></TableCell></TableRow> : items.length === 0 ? <TableRow><TableCell colSpan={ledger.columns.length + 1} align="center">データがありません。</TableCell></TableRow> : items.map((item) => <TableRow hover key={item.id}>{ledger.columns.map((column) => <TableCell key={column.key}>{displayLedgerValue(column.key, item[column.key])}</TableCell>)}<TableCell className="action-column"><Tooltip title="詳細・編集"><IconButton aria-label="詳細・編集" size="small" onClick={() => setEditing(item)}><EditOutlined fontSize="small" /></IconButton></Tooltip>{canEdit && <Tooltip title="削除"><IconButton aria-label="削除" size="small" color="error" onClick={() => setDeleting(item)}><DeleteOutline fontSize="small" /></IconButton></Tooltip>}</TableCell></TableRow>)}</TableBody></Table></TableContainer>{editing && <LedgerDialog ledger={ledger} initial={editing} canEdit={canEdit} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); reload() }} />}{deleting && <Dialog open onClose={() => setDeleting(null)}><DialogTitle>論理削除の確認</DialogTitle><DialogContent>「{display(deleting.name ?? deleting.site_name ?? deleting.full_name)}」を削除します。履歴は保持されます。</DialogContent><DialogActions><Button onClick={() => setDeleting(null)}>キャンセル</Button><Button color="error" variant="contained" onClick={remove}>削除する</Button></DialogActions></Dialog>}</Box>
}

function LedgerDialog({ ledger, initial, canEdit, onClose, onSaved }: { ledger: Ledger; initial: LedgerRecord; canEdit: boolean; onClose: () => void; onSaved: () => void }) {
  const [value, setValue] = useState<LedgerRecord>(initial); const [selectors, setSelectors] = useState<SelectorChoices>({ clients: [], employees: [], subcontractor_workers: [], projects: [] }); const [workers, setWorkers] = useState<AccountWorkerChoices>({ employees: [], subcontractor_workers: [] }); const [error, setError] = useState(''); const [saving, setSaving] = useState(false); const isNew = initial.id === 0
  useEffect(() => { if (!isNew) ledgerApi.get(ledger.resource, initial.id).then(({ item }) => setValue(item)).catch((err: Error) => setError(err.message)) }, [initial.id, isNew, ledger.resource])
  useEffect(() => { if (ledger.resource === 'projects') selectorApi.list().then(setSelectors).catch((err: Error) => setError(err.message)); if (ledger.resource === 'accounts') selectorApi.accountWorkerChoices().then(setWorkers).catch((err: Error) => setError(err.message)) }, [ledger.resource])
  const set = (key: string, next: string) => setValue((old) => ({ ...old, [key]: next }))
  const save = async () => { setSaving(true); setError(''); try { const payload = masterSavePayload(ledger.resource, value); if (isNew) await ledgerApi.create(ledger.resource, payload); else await ledgerApi.update(ledger.resource, value.id, payload); onSaved() } catch (err) { setError(err instanceof Error ? err.message : '保存できませんでした。') } finally { setSaving(false) } }
  const codeKey = generatedCodeKey(ledger.resource)
  const ordinaryFields = ledger.fields.filter(field => ledger.resource !== 'projects' || !['client_id', 'manager_employee_id'].includes(field.key))
  return <Dialog open fullWidth maxWidth="md" onClose={onClose}><DialogTitle>{isNew ? '新規登録' : '詳細・編集'}<IconButton aria-label="閉じる" onClick={onClose} sx={{ float: 'right' }}><Close /></IconButton></DialogTitle><DialogContent dividers>{error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}{ledger.resource === 'accounts' ? <AccountForm value={value} choices={workers} disabled={!canEdit} setValue={setValue} /> : <><Box className="form-grid">{!isNew && codeKey && <CodeField value={value[codeKey]} />}{ordinaryFields.map((field) => <FieldInput key={field.key} field={field} value={value[field.key]} disabled={!canEdit || (isMasked(field.key) && !isNew)} set={set} />)}{ledger.resource === 'projects' && <ProjectRelationFields value={value} choices={selectors} disabled={!canEdit} set={set} />}</Box><RelationshipEditor kind={ledger.relationship} value={value} choices={selectors} disabled={!canEdit} setValue={setValue} /></>}</DialogContent><DialogActions><Button onClick={onClose}>閉じる</Button>{canEdit && <Button variant="contained" color="secondary" startIcon={<SaveOutlined />} disabled={saving} onClick={save}>{saving ? '保存中…' : '保存'}</Button>}</DialogActions></Dialog>
}

function CodeField({ value }: { value: unknown }) { return <TextField fullWidth label="コード" value={String(value ?? '')} slotProps={{ input: { readOnly: true } }} /> }

function ProjectRelationFields({ value, choices, disabled, set }: { value: LedgerRecord; choices: SelectorChoices; disabled: boolean; set: (key: string, value: string) => void }) {
  const clientID = String(value.client_id ?? ''); const managerID = String(value.manager_employee_id ?? '')
  return <><FormControl fullWidth size="small"><InputLabel>得意先</InputLabel><Select label="得意先" value={clientID} disabled={disabled} onChange={event => set('client_id', event.target.value)}>{missingChoice(choices.clients, clientID, '削除済みの得意先')}{choices.clients.map(client => <MenuItem key={client.id} value={client.id} disabled={!isCurrentOrActiveChoice(client, clientID)}>{clientOptionLabel(client)}</MenuItem>)}</Select></FormControl><FormControl fullWidth size="small"><InputLabel>現場担当作業員</InputLabel><Select label="現場担当作業員" value={managerID} disabled={disabled} onChange={event => set('manager_employee_id', event.target.value)}><MenuItem value="">未設定</MenuItem>{missingChoice(choices.employees, managerID, '削除済みの現場担当作業員')}{choices.employees.map(employee => <MenuItem key={employee.id} value={employee.id} disabled={!isCurrentOrActiveChoice(employee, managerID)}>{employeeOptionLabel(employee)}</MenuItem>)}</Select></FormControl></>
}

function AccountForm({ value, choices, disabled, setValue }: { value: LedgerRecord; choices: AccountWorkerChoices; disabled: boolean; setValue: (value: LedgerRecord) => void }) {
  const workerType = value.worker_type === 'subcontractor' ? 'subcontractor' : 'employee'
  const workerID = value.worker_id ?? (workerType === 'employee' ? value.employee_id : value.subcontractor_worker_id) ?? ''
  const set = (key: string, next: string) => setValue({ ...value, [key]: next })
  const availableWorkers = workersForType(choices, workerType)
  return <Box className="form-grid"><TextField fullWidth label="ログインID" value={String(value.login_id ?? '')} disabled={disabled} onChange={event => set('login_id', event.target.value)} /><FormControl fullWidth size="small"><InputLabel>作業者区分</InputLabel><Select label="作業者区分" value={workerType} disabled={disabled} onChange={event => setValue({ ...value, worker_type: event.target.value, worker_id: '' })}><MenuItem value="employee">自社</MenuItem><MenuItem value="subcontractor">外注先</MenuItem></Select></FormControl><FormControl fullWidth size="small"><InputLabel>作業者</InputLabel><Select label="作業者" value={String(workerID)} disabled={disabled} onChange={event => set('worker_id', event.target.value)}>{missingChoice(availableWorkers, String(workerID), '削除済みの作業者')}{availableWorkers.map(worker => <MenuItem key={worker.id} value={worker.id} disabled={!isCurrentOrActiveChoice(worker, workerID)}>{'employee_code' in worker ? employeeOptionLabel(worker) : workerOptionLabel(worker)}</MenuItem>)}</Select></FormControl><TextField fullWidth label="パスワード（変更時のみ）" type="password" value={String(value.password ?? '')} disabled={disabled} onChange={event => set('password', event.target.value)} /><FieldInput field={{ key: 'role', label: '権限', type: 'select', choices: [['user', '一般'], ['admin', '管理者']] }} disabled={disabled} value={value.role ?? 'user'} set={set} /></Box>
}

function missingChoice(choices: Array<{ id: number }>, value: string, label: string) { return value !== '' && !choices.some(choice => choice.id === Number(value)) ? <MenuItem value={value} disabled>{label}</MenuItem> : null }

function FieldInput({ field, value, disabled, set }: { field: Field; value: unknown; disabled: boolean; set: (key: string, value: string) => void }) {
  if (field.type === 'select') return <FormControl fullWidth size="small"><InputLabel>{field.label}</InputLabel><Select label={field.label} value={String(value ?? '')} disabled={disabled} onChange={(event) => set(field.key, event.target.value)}>{field.choices?.map(([key, label]) => <MenuItem key={key} value={key}>{label}</MenuItem>)}</Select></FormControl>
  return <TextField fullWidth label={field.label} type={field.type === 'date' ? 'date' : 'text'} multiline={field.type === 'textarea'} minRows={field.type === 'textarea' ? 2 : undefined} InputLabelProps={{ shrink: shouldShrinkFieldLabel(field.type, value) }} value={String(value ?? '')} disabled={disabled} onChange={(event) => set(field.key, event.target.value)} />
}

export function RelationshipEditor({ kind, value, choices, disabled, setValue }: { kind?: Ledger['relationship']; value: LedgerRecord; choices: SelectorChoices; disabled: boolean; setValue: (value: LedgerRecord) => void }) {
  if (!kind) return null
  if (kind === 'member') return <ProjectMembersEditor value={value} choices={choices} disabled={disabled} setValue={setValue} />
  const listKey = kind === 'employee' ? 'qualifications' : 'workers'; const values = asRecords(value[listKey])
  const relationshipKind: RelationshipKind = kind === 'employee' ? 'qualification' : 'worker'
  const fields: Field[] = kind === 'employee' ? [{ key: 'qualification_name', label: '資格名' }, { key: 'acquired_on', label: '取得日', type: 'date' }, { key: 'renewed_on', label: '更新日', type: 'date' }, { key: 'notes', label: '資格備考' }] : [{ key: 'full_name', label: '外注従業員名' }, { key: 'work_description', label: '作業項目' }, { key: 'phone', label: '電話番号' }, { key: 'notes', label: '備考' }]
  const replace = (next: LedgerRecord[]) => setValue({ ...value, [listKey]: next })
  const update = (index: number, key: string, next: string) => replace(values.map((row, rowIndex) => rowIndex === index ? { ...row, [key]: next } : row))
  if (kind === 'employee' || kind === 'worker') return <Box className="detail-section">{kind === 'employee' && <WorkItemSelect value={value} disabled={disabled} setValue={setValue} />}<Box className="document-lines-heading"><Typography className="detail-heading">{kind === 'employee' ? '資格情報' : '外注先従業員'}</Typography>{!disabled && <Button size="small" startIcon={<Add />} onClick={() => replace(addRelationshipRow(values, relationshipKind) as LedgerRecord[])}>{kind === 'employee' ? '資格を追加' : '外注従業員を追加'}</Button>}</Box><TableContainer className="document-lines"><Table size="small"><TableHead><TableRow>{fields.map(field => <TableCell key={field.key} className={field.key === 'qualification_name' || field.key === 'full_name' ? 'relationship-primary-column' : undefined}>{field.label}</TableCell>)}<TableCell className="action-column" component="th" scope="col">操作</TableCell></TableRow></TableHead><TableBody>{values.map((row, index) => <TableRow key={relationshipRowKey(row, relationshipKind)}>{fields.map(field => <TableCell key={field.key} className={field.key === 'qualification_name' || field.key === 'full_name' ? 'relationship-primary-column' : undefined}><FieldInput field={field} disabled={disabled} value={row[field.key]} set={(key, next) => update(index, key, next)} /></TableCell>)}<TableCell className="action-column">{!disabled && <IconButton aria-label={`${kind === 'employee' ? '資格情報' : '外注先従業員'}を削除`} color="error" onClick={() => replace(values.filter((_, rowIndex) => rowIndex !== index))}><DeleteOutline /></IconButton>}</TableCell></TableRow>)}</TableBody></Table></TableContainer></Box>
  return null
}

const projectMemberFields: Field[] = [{ key: 'work_description', label: '作業項目' }, { key: 'start_on', label: '開始日', type: 'date' }, { key: 'end_on', label: '終了日', type: 'date' }, { key: 'phone', label: '連絡先' }, { key: 'notes', label: '備考' }]

function ProjectMembersEditor({ value, choices, disabled, setValue }: { value: LedgerRecord; choices: SelectorChoices; disabled: boolean; setValue: (value: LedgerRecord) => void }) {
  const members = asRecords(value.members); const [composer, setComposer] = useState(() => changeProjectMemberType('employee')); const labelPrefix = useId()
  const availableWorkers = projectMemberChoices(choices, composer.memberType)
  const replace = (next: LedgerRecord[]) => setValue({ ...value, members: next })
  const update = (index: number, next: LedgerRecord) => replace(updateProjectMember(members, index, next) as LedgerRecord[])
  const remove = (index: number) => replace(removeProjectMember(members, index) as LedgerRecord[])
  const add = () => { if (composer.workerID !== '') { replace(addProjectMember(members, composer.memberType, composer.workerID) as LedgerRecord[]); setComposer(changeProjectMemberType(composer.memberType)) } }
  const typeLabelID = `${labelPrefix}-member-type-label`; const workerLabelID = `${labelPrefix}-member-worker-label`
  return <Box className="detail-section project-members-section"><Typography className="detail-heading">案件担当員</Typography>{!disabled && <Box className="project-member-add"><FormControl size="small"><InputLabel id={typeLabelID}>作業者区分</InputLabel><Select id={`${labelPrefix}-member-type`} labelId={typeLabelID} label="作業者区分" value={composer.memberType} onChange={event => setComposer(changeProjectMemberType(event.target.value as ProjectMemberType))}><MenuItem value="employee">自社</MenuItem><MenuItem value="subcontractor">外注先</MenuItem></Select></FormControl><FormControl size="small" fullWidth><InputLabel id={workerLabelID}>従業員</InputLabel><Select id={`${labelPrefix}-member-worker`} labelId={workerLabelID} label="従業員" value={composer.workerID} onChange={event => setComposer(current => ({ ...current, workerID: event.target.value }))}>{availableWorkers.map(worker => <MenuItem key={worker.id} value={String(worker.id)} disabled={!isCurrentOrActiveChoice(worker, '')}>{'employee_code' in worker ? employeeOptionLabel(worker) : workerOptionLabel(worker)}</MenuItem>)}</Select></FormControl><Button variant="outlined" startIcon={<Add />} disabled={composer.workerID === ''} onClick={add}>追加</Button></Box>}<ProjectMembersTable title="自社従業員" memberType="employee" members={members} choices={choices} disabled={disabled} update={update} remove={remove} /><ProjectMembersTable title="外注先従業員" memberType="subcontractor" members={members} choices={choices} disabled={disabled} update={update} remove={remove} /></Box>
}

export function ProjectMembersTable({ title, memberType, members, choices, disabled, update, remove }: { title: string; memberType: ProjectMemberType; members: LedgerRecord[]; choices: SelectorChoices; disabled: boolean; update: (index: number, member: LedgerRecord) => void; remove: (index: number) => void }) {
  const rows = projectMemberRows(members, memberType); const labelPrefix = useId()
  const availableWorkers = projectMemberChoices(choices, memberType); const idKey = memberType === 'employee' ? 'employee_id' : 'subcontractor_worker_id'
  return <Box className="project-member-table"><Typography className="project-member-table-title">{title}</Typography><TableContainer className="document-lines"><Table size="small"><TableHead><TableRow><TableCell>従業員</TableCell>{projectMemberFields.map(field => <TableCell key={field.key}>{field.label}</TableCell>)}{!disabled && <TableCell className="project-member-action" component="th" scope="col">操作</TableCell>}</TableRow></TableHead><TableBody>{rows.length === 0 ? <TableRow><TableCell colSpan={projectMemberFields.length + (disabled ? 1 : 2)} align="center">担当員は登録されていません。</TableCell></TableRow> : rows.map(({ key, member, index }) => { const currentID = String(member[idKey] ?? ''); const workerLabelID = `${labelPrefix}-member-worker-${key}-label`; return <TableRow key={key}><TableCell><FormControl fullWidth size="small"><InputLabel id={workerLabelID}>従業員</InputLabel><Select id={`${labelPrefix}-member-worker-${key}`} labelId={workerLabelID} label="従業員" value={currentID} disabled={disabled} onChange={event => update(index, projectMemberSelection(memberType, event.target.value, member) as LedgerRecord)}>{missingChoice(availableWorkers, currentID, '削除済みの従業員')}{availableWorkers.map(worker => <MenuItem key={worker.id} value={String(worker.id)} disabled={!isCurrentOrActiveChoice(worker, currentID)}>{'employee_code' in worker ? employeeOptionLabel(worker) : workerOptionLabel(worker)}</MenuItem>)}</Select></FormControl></TableCell>{projectMemberFields.map(field => <TableCell key={field.key}><FieldInput field={field} disabled={disabled} value={member[field.key]} set={(fieldKey, next) => update(index, { ...member, [fieldKey]: next } as LedgerRecord)} /></TableCell>)}{!disabled && <TableCell className="project-member-action"><IconButton aria-label={`${title}を削除`} color="error" onClick={() => remove(index)}><DeleteOutline /></IconButton></TableCell>}</TableRow> })}</TableBody></Table></TableContainer></Box>
}

function WorkItemSelect({ value, disabled, setValue }: { value: LedgerRecord; disabled: boolean; setValue: (value: LedgerRecord) => void }) { const [items, setItems] = useState<LedgerRecord[]>([]); useEffect(() => { ledgerApi.list('master-items', '?type=work').then(({ items: next }) => setItems(next)).catch(() => undefined) }, []); const selected = asRecords(value.workItemIds).map((item) => Number(item.id)).filter(Boolean); const primitive = Array.isArray(value.workItemIds) ? value.workItemIds.map((id) => Number(id)).filter(Boolean) : selected; return <FormControl fullWidth size="small" sx={{ mt: 2 }}><InputLabel>作業項目（複数選択）</InputLabel><Select multiple label="作業項目（複数選択）" disabled={disabled} value={primitive} onChange={(event) => setValue({ ...value, workItemIds: event.target.value as number[] })}>{items.map((item) => <MenuItem key={item.id} value={item.id}>{String(item.name)}</MenuItem>)}</Select></FormControl> }

function asRecords(value: unknown): LedgerRecord[] { return Array.isArray(value) ? value.filter((entry): entry is LedgerRecord => entry !== null && typeof entry === 'object') : [] }
function display(value: unknown): string { return value === null || value === undefined || value === '' ? '—' : String(value) }
function isMasked(key: string) { return ['employment_insurance_no', 'pension_no', 'insurance_card_no'].includes(key) }

-- Forward-only convergence for databases that had already recorded migration
-- 000007 before its demo payload was finalized. Markers make both reruns and
-- rollback precise; no business row is selected by date alone.
ALTER TABLE accounts ADD COLUMN seed_marker VARCHAR(64) NULL, ADD KEY idx_accounts_seed_marker (seed_marker);
ALTER TABLE subcontractor_workers ADD COLUMN seed_marker VARCHAR(64) NULL, ADD KEY idx_subcontractor_workers_seed_marker (seed_marker);
ALTER TABLE project_members ADD COLUMN seed_marker VARCHAR(64) NULL, ADD KEY idx_project_members_seed_marker (seed_marker);
ALTER TABLE daily_reports ADD COLUMN seed_marker VARCHAR(64) NULL, ADD KEY idx_daily_reports_seed_marker (seed_marker);
ALTER TABLE schedules ADD COLUMN seed_marker VARCHAR(64) NULL, ADD KEY idx_schedules_seed_marker (seed_marker);
ALTER TABLE estimates ADD COLUMN seed_marker VARCHAR(64) NULL, ADD KEY idx_estimates_seed_marker (seed_marker);
ALTER TABLE invoices ADD COLUMN seed_marker VARCHAR(64) NULL, ADD KEY idx_invoices_seed_marker (seed_marker);

UPDATE accounts a JOIN employees e ON e.id=a.employee_id SET a.seed_marker='T6-DEMO' WHERE e.employee_code='EMP-0002' AND a.login_id='misaki' AND a.role='user';
UPDATE subcontractor_workers w JOIN subcontractors s ON s.id=w.subcontractor_id SET w.seed_marker='T6-DEMO' WHERE s.subcontractor_code='SUB-0001' AND w.full_name='高橋 直人' AND w.work_description='設備補助' AND w.phone='090-3456-7890' AND w.notes='架空デモ作業員';
UPDATE project_members pm JOIN projects p ON p.id=pm.project_id JOIN employees e ON e.id=pm.employee_id SET pm.seed_marker='T6-DEMO' WHERE p.project_number='PRJ-2026-001' AND e.employee_code='EMP-0001' AND pm.work_description='現場管理' AND pm.start_on='2026-07-15' AND pm.end_on='2026-09-30';
UPDATE daily_reports dr JOIN employees e ON e.id=dr.employee_id JOIN projects p ON p.id=dr.project_id SET dr.seed_marker='T6-DEMO' WHERE e.employee_code='EMP-0001' AND p.project_number='PRJ-2026-001' AND dr.report_date='2026-07-25' AND dr.started_at='08:00' AND dr.ended_at='17:00' AND dr.break_minutes=60 AND dr.work_description='空調配管の施工確認' AND dr.revision=1;
UPDATE schedules s JOIN employees e ON e.id=s.employee_id JOIN master_items mi ON mi.id=s.master_item_id SET s.seed_marker='T6-DEMO' WHERE e.employee_code='EMP-0001' AND s.schedule_date='2026-07-25' AND ((s.slot=1 AND mi.item_code='SCH-001') OR (s.slot=2 AND mi.item_code='SCH-002')) AND s.revision=1;
UPDATE estimates SET seed_marker='T6-DEMO' WHERE estimate_number='DEM-EST-2026-001';
UPDATE invoices SET seed_marker='T6-DEMO' WHERE invoice_number='DEM-INV-2026-001';

INSERT INTO accounts (employee_id, login_id, password_hash, role, seed_marker)
SELECT e.id, 'misaki', '$2y$12$7jbHG/IzCyVxPD.A5emlveCU4VMFQ.EZIgAiUImlGXR6ObtoPc6XW', 'user', 'T6-DEMO' FROM employees e WHERE e.employee_code='EMP-0002' AND NOT EXISTS (SELECT 1 FROM accounts a WHERE a.login_id='misaki');
INSERT INTO subcontractor_workers (subcontractor_id, full_name, work_description, phone, notes, seed_marker)
SELECT s.id, '高橋 直人', '設備補助', '090-3456-7890', '架空デモ作業員', 'T6-DEMO' FROM subcontractors s WHERE s.subcontractor_code='SUB-0001' AND NOT EXISTS (SELECT 1 FROM subcontractor_workers w WHERE w.subcontractor_id=s.id AND w.full_name='高橋 直人' AND w.work_description='設備補助' AND w.phone='090-3456-7890' AND w.notes='架空デモ作業員');
INSERT INTO project_members (project_id, employee_id, work_description, start_on, end_on, phone, seed_marker)
SELECT p.id, e.id, '現場管理', '2026-07-15', '2026-09-30', e.phone, 'T6-DEMO' FROM projects p JOIN employees e ON e.employee_code='EMP-0001' WHERE p.project_number='PRJ-2026-001' AND NOT EXISTS (SELECT 1 FROM project_members pm WHERE pm.project_id=p.id AND pm.employee_id=e.id AND pm.work_description='現場管理' AND pm.start_on='2026-07-15' AND pm.end_on='2026-09-30');
INSERT INTO daily_reports (employee_id, report_date, project_id, client_id, started_at, ended_at, break_minutes, work_description, revision, seed_marker)
SELECT e.id, '2026-07-25', p.id, c.id, '08:00', '17:00', 60, '空調配管の施工確認', 1, 'T6-DEMO' FROM employees e JOIN projects p ON p.project_number='PRJ-2026-001' JOIN clients c ON c.client_code='CLI-0001' WHERE e.employee_code='EMP-0001' AND NOT EXISTS (SELECT 1 FROM daily_reports dr WHERE dr.employee_id=e.id AND dr.report_date='2026-07-25' AND dr.project_id=p.id AND dr.client_id=c.id AND dr.started_at='08:00' AND dr.ended_at='17:00' AND dr.break_minutes=60 AND dr.work_description='空調配管の施工確認' AND dr.revision=1);
INSERT INTO schedules (employee_id, schedule_date, slot, master_item_id, revision, seed_marker)
SELECT e.id, '2026-07-25', 1, mi.id, 1, 'T6-DEMO' FROM employees e JOIN master_items mi ON mi.item_code='SCH-001' WHERE e.employee_code='EMP-0001' AND NOT EXISTS (SELECT 1 FROM schedules s WHERE s.employee_id=e.id AND s.schedule_date='2026-07-25' AND s.slot=1);
INSERT INTO schedules (employee_id, schedule_date, slot, master_item_id, revision, seed_marker)
SELECT e.id, '2026-07-25', 2, mi.id, 1, 'T6-DEMO' FROM employees e JOIN master_items mi ON mi.item_code='SCH-002' WHERE e.employee_code='EMP-0001' AND NOT EXISTS (SELECT 1 FROM schedules s WHERE s.employee_id=e.id AND s.schedule_date='2026-07-25' AND s.slot=2);
INSERT INTO estimates (estimate_number, project_id, issued_on, contractor_name, contractor_representative, contractor_address, status, discount_amount, seed_marker)
SELECT 'DEM-EST-2026-001', p.id, '2026-07-15', 'コンストラクションデモ株式会社', '田中 恒一', '東京都江東区潮見1-2-3', 'confirmed', 0, 'T6-DEMO' FROM projects p WHERE p.project_number='PRJ-2026-001' AND NOT EXISTS (SELECT 1 FROM estimates e WHERE e.seed_marker='T6-DEMO' AND e.estimate_number='DEM-EST-2026-001');
INSERT INTO estimate_lines (estimate_id, line_no, item_name, quantity, unit, unit_price, notes)
SELECT e.id, 1, '配管施工費', 1, '式', 480000, '架空デモ見積' FROM estimates e WHERE e.seed_marker='T6-DEMO' AND NOT EXISTS (SELECT 1 FROM estimate_lines l WHERE l.estimate_id=e.id AND l.line_no=1);
INSERT INTO invoices (invoice_number, project_id, issued_on, billing_type, contractor_name, contractor_representative, contractor_address, discount_amount, seed_marker)
SELECT 'DEM-INV-2026-001', p.id, '2026-07-31', 'individual', 'コンストラクションデモ株式会社', '田中 恒一', '東京都江東区潮見1-2-3', 0, 'T6-DEMO' FROM projects p WHERE p.project_number='PRJ-2026-001' AND NOT EXISTS (SELECT 1 FROM invoices i WHERE i.seed_marker='T6-DEMO' AND i.invoice_number='DEM-INV-2026-001');
INSERT INTO invoice_lines (invoice_id, line_no, item_name, quantity, unit, unit_price, notes)
SELECT i.id, 1, '7月施工出来高', 1, '式', 240000, '架空デモ請求' FROM invoices i WHERE i.seed_marker='T6-DEMO' AND NOT EXISTS (SELECT 1 FROM invoice_lines l WHERE l.invoice_id=i.id AND l.line_no=1);
DELETE el FROM estimate_lines el JOIN estimates e ON e.id=el.estimate_id WHERE e.estimate_number IN ('EST-T4-001','EST-T4-PAGES');
DELETE il FROM invoice_lines il JOIN invoices i ON i.id=il.invoice_id WHERE i.invoice_number IN ('EST-T4-001','EST-T4-PAGES');
DELETE FROM estimates WHERE estimate_number IN ('EST-T4-001','EST-T4-PAGES');
DELETE FROM invoices WHERE invoice_number IN ('EST-T4-001','EST-T4-PAGES');

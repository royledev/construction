INSERT INTO employees (employee_code, full_name, full_name_kana, occupation, hired_on, phone) VALUES
  ('EMP-0001', '田中 恒一', 'タナカ コウイチ', '現場監督', '2021-04-01', '090-1234-5678'),
  ('EMP-0002', '佐藤 美咲', 'サトウ ミサキ', '作業員', '2023-04-01', '090-2345-6789');

-- Demo credentials: admin / construction-demo
INSERT INTO accounts (employee_id, login_id, password_hash, role) VALUES
  (1, 'admin', '$2y$12$7jbHG/IzCyVxPD.A5emlveCU4VMFQ.EZIgAiUImlGXR6ObtoPc6XW', 'admin');

INSERT INTO clients (client_code, name, contact_name, contact_phone, address) VALUES
  ('CLI-0001', '青空建設株式会社', '鈴木 一郎', '03-1234-5678', '東京都江東区潮見1-2-3');
INSERT INTO subcontractors (subcontractor_code, name, contact_name, contact_phone) VALUES
  ('SUB-0001', '東都設備有限会社', '高橋 健', '03-9876-5432');
INSERT INTO master_items (item_type, item_code, name) VALUES
  ('work', 'WORK-001', '配管工事'), ('estimate_invoice', 'EI-001', '施工費'), ('unit_price', 'UP-001', '普通作業員'),
  ('schedule', 'SCH-001', '現場作業'), ('schedule', 'SCH-002', '資材搬入');
INSERT INTO projects (project_number, site_name, site_address, client_id, manager_employee_id, ordered_on, construction_start_on, construction_end_on, status) VALUES
  ('PRJ-2026-001', '潮見オフィス改修工事', '東京都江東区潮見1-2-3', 1, 1, '2026-07-01', '2026-07-15', '2026-09-30', 'ordered');

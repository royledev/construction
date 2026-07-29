-- Original v7 payload. Additions made after v7 reached existing installations
-- live in 000008, because golang-migrate never re-runs an applied version.
INSERT IGNORE INTO accounts (employee_id, login_id, password_hash, role)
SELECT id, 'misaki', '$2y$12$7jbHG/IzCyVxPD.A5emlveCU4VMFQ.EZIgAiUImlGXR6ObtoPc6XW', 'user' FROM employees WHERE employee_code = 'EMP-0002';
INSERT IGNORE INTO schedules (employee_id, schedule_date, slot, master_item_id, revision)
SELECT e.id, '2026-07-25', 1, mi.id, 1 FROM employees e JOIN master_items mi ON mi.item_code='SCH-001' WHERE e.employee_code='EMP-0001';
INSERT IGNORE INTO schedules (employee_id, schedule_date, slot, master_item_id, revision)
SELECT e.id, '2026-07-25', 2, mi.id, 1 FROM employees e JOIN master_items mi ON mi.item_code='SCH-002' WHERE e.employee_code='EMP-0001';

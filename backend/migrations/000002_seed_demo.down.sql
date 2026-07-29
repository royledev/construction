DELETE FROM projects WHERE project_number = 'PRJ-2026-001';
DELETE FROM master_items WHERE item_code IN ('WORK-001', 'EI-001', 'UP-001', 'SCH-001', 'SCH-002');
DELETE FROM subcontractors WHERE subcontractor_code = 'SUB-0001';
DELETE FROM clients WHERE client_code = 'CLI-0001';
DELETE FROM accounts WHERE login_id = 'admin';
DELETE FROM employees WHERE employee_code IN ('EMP-0001', 'EMP-0002');

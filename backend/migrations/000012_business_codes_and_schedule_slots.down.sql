ALTER TABLE accounts DROP INDEX uq_accounts_active_worker, DROP COLUMN active_worker_key, DROP COLUMN worker_id, DROP COLUMN worker_type;
DROP TABLE IF EXISTS schedule_slots;
DROP TABLE IF EXISTS organization_profiles;

-- Restore numeric primary keys before legacy codes.  In particular, v11
-- permits the same item_code in different master-item types, while v12 makes
-- item_code the physical primary key.
ALTER TABLE master_items DROP PRIMARY KEY, DROP INDEX uq_master_items_id, ADD PRIMARY KEY (id);
ALTER TABLE subcontractors DROP PRIMARY KEY, DROP INDEX uq_subcontractors_id, ADD PRIMARY KEY (id);
ALTER TABLE clients DROP PRIMARY KEY, DROP INDEX uq_clients_id, ADD PRIMARY KEY (id);
ALTER TABLE employees DROP PRIMARY KEY, DROP INDEX uq_employees_id, ADD PRIMARY KEY (id);
ALTER TABLE invoices DROP PRIMARY KEY, DROP INDEX uq_invoices_id, ADD PRIMARY KEY (id);
ALTER TABLE estimates DROP PRIMARY KEY, DROP INDEX uq_estimates_id, ADD PRIMARY KEY (id);
ALTER TABLE projects DROP PRIMARY KEY, DROP INDEX uq_projects_id, ADD PRIMARY KEY (id);

UPDATE projects p JOIN business_code_backups b ON b.table_name='projects' AND b.internal_id=p.id SET p.project_code=b.original_code;
UPDATE estimates p JOIN business_code_backups b ON b.table_name='estimates' AND b.internal_id=p.id SET p.estimate_code=b.original_code;
UPDATE invoices p JOIN business_code_backups b ON b.table_name='invoices' AND b.internal_id=p.id SET p.invoice_code=b.original_code;
UPDATE employees p JOIN business_code_backups b ON b.table_name='employees' AND b.internal_id=p.id SET p.employee_code=b.original_code;
UPDATE clients p JOIN business_code_backups b ON b.table_name='clients' AND b.internal_id=p.id SET p.client_code=b.original_code;
UPDATE subcontractors p JOIN business_code_backups b ON b.table_name='subcontractors' AND b.internal_id=p.id SET p.subcontractor_code=b.original_code;
UPDATE master_items p JOIN business_code_backups b ON b.table_name='master_items' AND b.internal_id=p.id SET p.item_code=b.original_code;

ALTER TABLE projects DROP INDEX uq_projects_code, ADD UNIQUE KEY uq_projects_number (project_code);
ALTER TABLE estimates DROP INDEX uq_estimates_code, ADD UNIQUE KEY uq_estimates_number (estimate_code);
ALTER TABLE invoices DROP INDEX uq_invoices_code, ADD UNIQUE KEY uq_invoices_number (invoice_code);
ALTER TABLE projects RENAME COLUMN project_code TO project_number;
ALTER TABLE estimates RENAME COLUMN estimate_code TO estimate_number;
ALTER TABLE invoices RENAME COLUMN invoice_code TO invoice_number;
DROP TABLE IF EXISTS business_code_backups;

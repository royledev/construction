-- Public business identifiers are immutable codes.  Numeric ids remain unique
-- internal references so existing foreign keys do not need to be rewritten.
ALTER TABLE projects RENAME COLUMN project_number TO project_code;
ALTER TABLE estimates RENAME COLUMN estimate_number TO estimate_code;
ALTER TABLE invoices RENAME COLUMN invoice_number TO invoice_code;

CREATE TABLE business_code_backups (table_name VARCHAR(32) NOT NULL, internal_id BIGINT UNSIGNED NOT NULL, original_code VARCHAR(32) NOT NULL, PRIMARY KEY (table_name, internal_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
INSERT INTO business_code_backups SELECT 'projects', id, project_code FROM projects;
INSERT INTO business_code_backups SELECT 'estimates', id, estimate_code FROM estimates;
INSERT INTO business_code_backups SELECT 'invoices', id, invoice_code FROM invoices;
INSERT INTO business_code_backups SELECT 'employees', id, employee_code FROM employees;
INSERT INTO business_code_backups SELECT 'clients', id, client_code FROM clients;
INSERT INTO business_code_backups SELECT 'subcontractors', id, subcontractor_code FROM subcontractors;
INSERT INTO business_code_backups SELECT 'master_items', id, item_code FROM master_items;

ALTER TABLE projects DROP INDEX uq_projects_number, ADD UNIQUE KEY uq_projects_code (project_code);
ALTER TABLE estimates DROP INDEX uq_estimates_number, ADD UNIQUE KEY uq_estimates_code (estimate_code);
ALTER TABLE invoices DROP INDEX uq_invoices_number, ADD UNIQUE KEY uq_invoices_code (invoice_code);

UPDATE projects SET project_code = CONCAT('PRJ-', LPAD(id, 6, '0'));
UPDATE estimates SET estimate_code = CONCAT('EST-', LPAD(id, 6, '0'));
UPDATE invoices SET invoice_code = CONCAT('INV-', LPAD(id, 6, '0'));
UPDATE employees SET employee_code = CONCAT('EMP-', LPAD(id, 6, '0'));
UPDATE clients SET client_code = CONCAT('CLI-', LPAD(id, 6, '0'));
UPDATE subcontractors SET subcontractor_code = CONCAT('SUB-', LPAD(id, 6, '0'));
UPDATE master_items SET item_code = CONCAT(CASE item_type WHEN 'work' THEN 'WORK' WHEN 'estimate_invoice' THEN 'EI' WHEN 'unit_price' THEN 'UP' ELSE 'SCH' END, '-', LPAD(id, 6, '0'));

-- AUTO_INCREMENT is retained through a unique internal-key index; each business
-- code is the physical primary key requested by the public data model.
ALTER TABLE projects DROP PRIMARY KEY, ADD PRIMARY KEY (project_code), ADD UNIQUE KEY uq_projects_id (id);
ALTER TABLE estimates DROP PRIMARY KEY, ADD PRIMARY KEY (estimate_code), ADD UNIQUE KEY uq_estimates_id (id);
ALTER TABLE invoices DROP PRIMARY KEY, ADD PRIMARY KEY (invoice_code), ADD UNIQUE KEY uq_invoices_id (id);
ALTER TABLE employees DROP PRIMARY KEY, ADD PRIMARY KEY (employee_code), ADD UNIQUE KEY uq_employees_id (id);
ALTER TABLE clients DROP PRIMARY KEY, ADD PRIMARY KEY (client_code), ADD UNIQUE KEY uq_clients_id (id);
ALTER TABLE subcontractors DROP PRIMARY KEY, ADD PRIMARY KEY (subcontractor_code), ADD UNIQUE KEY uq_subcontractors_id (id);
ALTER TABLE master_items DROP PRIMARY KEY, ADD PRIMARY KEY (item_code), ADD UNIQUE KEY uq_master_items_id (id);

CREATE TABLE organization_profiles (
  id TINYINT UNSIGNED NOT NULL,
  company_name VARCHAR(160) NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id), CONSTRAINT chk_organization_profile_singleton CHECK (id = 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_ja_0900_as_cs;
INSERT INTO organization_profiles (id, company_name) VALUES (1, 'コンストラクションデモ株式会社');

CREATE TABLE schedule_slots (
  slot SMALLINT UNSIGNED NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (slot)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_ja_0900_as_cs;
INSERT INTO schedule_slots (slot) VALUES (1), (2);
INSERT IGNORE INTO schedule_slots (slot) SELECT DISTINCT slot FROM schedules WHERE slot >= 3;

-- MySQL unique keys treat NULL as distinct.  This generated value gives active
-- worker assignments a real uniqueness guarantee while allowing soft deletion.
ALTER TABLE accounts
  ADD COLUMN worker_type VARCHAR(16) GENERATED ALWAYS AS (CASE WHEN employee_id IS NULL THEN 'subcontractor' ELSE 'employee' END) STORED,
  ADD COLUMN worker_id BIGINT UNSIGNED GENERATED ALWAYS AS (COALESCE(employee_id, subcontractor_worker_id)) STORED,
  ADD COLUMN active_worker_key VARCHAR(64) GENERATED ALWAYS AS (CASE WHEN deleted_at IS NULL THEN CONCAT(worker_type, ':', worker_id) ELSE NULL END) STORED,
  ADD UNIQUE KEY uq_accounts_active_worker (active_worker_key);

CREATE TABLE employees (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  employee_code VARCHAR(32) NOT NULL,
  status ENUM('active', 'retired') NOT NULL DEFAULT 'active',
  full_name VARCHAR(120) NOT NULL,
  full_name_kana VARCHAR(120) NULL,
  occupation VARCHAR(100) NULL,
  birth_date DATE NULL,
  hired_on DATE NULL,
  retired_on DATE NULL,
  employment_insurance_no VARCHAR(64) NULL,
  pension_no VARCHAR(64) NULL,
  insurance_card_no VARCHAR(64) NULL,
  my_number_encrypted VARBINARY(512) NULL,
  postal_code VARCHAR(16) NULL,
  address VARCHAR(255) NULL,
  phone VARCHAR(32) NULL,
  deleted_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id), UNIQUE KEY uq_employees_code (employee_code), KEY idx_employees_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_ja_0900_as_cs;

CREATE TABLE accounts (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  employee_id BIGINT UNSIGNED NOT NULL,
  login_id VARCHAR(80) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  role ENUM('admin', 'user') NOT NULL DEFAULT 'user',
  deleted_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id), UNIQUE KEY uq_accounts_login_id (login_id), KEY idx_accounts_employee_id (employee_id),
  CONSTRAINT fk_accounts_employee FOREIGN KEY (employee_id) REFERENCES employees (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_ja_0900_as_cs;

CREATE TABLE clients (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  client_code VARCHAR(32) NOT NULL,
  status ENUM('active', 'inactive') NOT NULL DEFAULT 'active',
  name VARCHAR(160) NOT NULL,
  honorific VARCHAR(16) NOT NULL DEFAULT '御中', kana VARCHAR(160) NULL,
  phone VARCHAR(32) NULL, fax VARCHAR(32) NULL, email VARCHAR(255) NULL,
  postal_code VARCHAR(16) NULL, address VARCHAR(255) NULL,
  contact_name VARCHAR(120) NULL, contact_phone VARCHAR(32) NULL,
  registration_number VARCHAR(32) NULL, closing_day TINYINT UNSIGNED NULL,
  payment_due_rule VARCHAR(100) NULL, payment_method VARCHAR(80) NULL, payment_site_days SMALLINT UNSIGNED NULL,
  bank_name VARCHAR(120) NULL, bank_branch VARCHAR(120) NULL, bank_account_no VARCHAR(64) NULL,
  notes TEXT NULL, deleted_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id), UNIQUE KEY uq_clients_code (client_code), KEY idx_clients_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_ja_0900_as_cs;

CREATE TABLE subcontractors (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  subcontractor_code VARCHAR(32) NOT NULL,
  status ENUM('active', 'inactive') NOT NULL DEFAULT 'active', name VARCHAR(160) NOT NULL,
  honorific VARCHAR(16) NOT NULL DEFAULT '御中', kana VARCHAR(160) NULL,
  phone VARCHAR(32) NULL, fax VARCHAR(32) NULL, email VARCHAR(255) NULL, postal_code VARCHAR(16) NULL, address VARCHAR(255) NULL,
  contact_name VARCHAR(120) NULL, contact_phone VARCHAR(32) NULL, registration_number VARCHAR(32) NULL,
  payment_method VARCHAR(80) NULL, payment_site_days SMALLINT UNSIGNED NULL, bank_name VARCHAR(120) NULL, bank_branch VARCHAR(120) NULL, bank_account_no VARCHAR(64) NULL,
  notes TEXT NULL, deleted_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id), UNIQUE KEY uq_subcontractors_code (subcontractor_code), KEY idx_subcontractors_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_ja_0900_as_cs;

CREATE TABLE subcontractor_workers (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, subcontractor_id BIGINT UNSIGNED NOT NULL,
  full_name VARCHAR(120) NOT NULL, work_description VARCHAR(255) NULL, phone VARCHAR(32) NULL, notes TEXT NULL, deleted_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id), KEY idx_subcontractor_workers_subcontractor (subcontractor_id),
  CONSTRAINT fk_subcontractor_workers_company FOREIGN KEY (subcontractor_id) REFERENCES subcontractors (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_ja_0900_as_cs;

CREATE TABLE master_items (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, item_type ENUM('work', 'estimate_invoice', 'unit_price', 'schedule') NOT NULL,
  item_code VARCHAR(32) NOT NULL, name VARCHAR(160) NOT NULL, notes TEXT NULL, deleted_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id), UNIQUE KEY uq_master_items_type_code (item_type, item_code), KEY idx_master_items_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_ja_0900_as_cs;

CREATE TABLE client_unit_prices (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, client_id BIGINT UNSIGNED NOT NULL, master_item_id BIGINT UNSIGNED NOT NULL, unit_price DECIMAL(14,2) NOT NULL,
  deleted_at DATETIME NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id), UNIQUE KEY uq_client_unit_prices (client_id, master_item_id),
  CONSTRAINT fk_client_unit_prices_client FOREIGN KEY (client_id) REFERENCES clients (id),
  CONSTRAINT fk_client_unit_prices_item FOREIGN KEY (master_item_id) REFERENCES master_items (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_ja_0900_as_cs;

CREATE TABLE projects (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, project_number VARCHAR(32) NOT NULL, site_name VARCHAR(160) NOT NULL, site_address VARCHAR(255) NULL,
  client_id BIGINT UNSIGNED NOT NULL, manager_employee_id BIGINT UNSIGNED NULL, ordered_on DATE NULL, construction_start_on DATE NULL, construction_end_on DATE NULL,
  completed_on DATE NULL, lost_on DATE NULL, status ENUM('unreceived', 'ordered', 'completed', 'lost') NOT NULL DEFAULT 'unreceived', notes TEXT NULL, deleted_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id), UNIQUE KEY uq_projects_number (project_number), KEY idx_projects_client (client_id), KEY idx_projects_deleted_at (deleted_at),
  CONSTRAINT fk_projects_client FOREIGN KEY (client_id) REFERENCES clients (id), CONSTRAINT fk_projects_manager FOREIGN KEY (manager_employee_id) REFERENCES employees (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_ja_0900_as_cs;

CREATE TABLE project_members (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, project_id BIGINT UNSIGNED NOT NULL, employee_id BIGINT UNSIGNED NULL, subcontractor_worker_id BIGINT UNSIGNED NULL,
  work_description VARCHAR(255) NULL, start_on DATE NULL, end_on DATE NULL, phone VARCHAR(32) NULL, notes TEXT NULL, deleted_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id), KEY idx_project_members_project (project_id),
  CONSTRAINT fk_project_members_project FOREIGN KEY (project_id) REFERENCES projects (id), CONSTRAINT fk_project_members_employee FOREIGN KEY (employee_id) REFERENCES employees (id),
  CONSTRAINT fk_project_members_subworker FOREIGN KEY (subcontractor_worker_id) REFERENCES subcontractor_workers (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_ja_0900_as_cs;

CREATE TABLE estimates (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, estimate_number VARCHAR(32) NOT NULL, project_id BIGINT UNSIGNED NOT NULL, issued_on DATE NOT NULL,
  contractor_name VARCHAR(160) NULL, contractor_representative VARCHAR(120) NULL, contractor_address VARCHAR(255) NULL, status ENUM('draft', 'confirmed') NOT NULL DEFAULT 'draft',
  discount_amount DECIMAL(14,2) NOT NULL DEFAULT 0, deleted_at DATETIME NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id), UNIQUE KEY uq_estimates_number (estimate_number), CONSTRAINT fk_estimates_project FOREIGN KEY (project_id) REFERENCES projects (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_ja_0900_as_cs;

CREATE TABLE estimate_lines (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, estimate_id BIGINT UNSIGNED NOT NULL, line_no INT UNSIGNED NOT NULL, item_name VARCHAR(160) NOT NULL,
  quantity DECIMAL(14,3) NOT NULL DEFAULT 0, unit VARCHAR(24) NULL, unit_price DECIMAL(14,2) NOT NULL DEFAULT 0, notes TEXT NULL,
  PRIMARY KEY (id), UNIQUE KEY uq_estimate_lines_no (estimate_id, line_no), CONSTRAINT fk_estimate_lines_estimate FOREIGN KEY (estimate_id) REFERENCES estimates (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_ja_0900_as_cs;

CREATE TABLE invoices (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, invoice_number VARCHAR(32) NOT NULL, project_id BIGINT UNSIGNED NOT NULL, issued_on DATE NOT NULL, billing_type ENUM('closing', 'individual') NOT NULL,
  contractor_name VARCHAR(160) NULL, contractor_representative VARCHAR(120) NULL, contractor_address VARCHAR(255) NULL, discount_amount DECIMAL(14,2) NOT NULL DEFAULT 0,
  deleted_at DATETIME NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id), UNIQUE KEY uq_invoices_number (invoice_number), CONSTRAINT fk_invoices_project FOREIGN KEY (project_id) REFERENCES projects (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_ja_0900_as_cs;

CREATE TABLE invoice_lines (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, invoice_id BIGINT UNSIGNED NOT NULL, line_no INT UNSIGNED NOT NULL, item_name VARCHAR(160) NOT NULL,
  quantity DECIMAL(14,3) NOT NULL DEFAULT 0, unit VARCHAR(24) NULL, unit_price DECIMAL(14,2) NOT NULL DEFAULT 0, notes TEXT NULL,
  PRIMARY KEY (id), UNIQUE KEY uq_invoice_lines_no (invoice_id, line_no), CONSTRAINT fk_invoice_lines_invoice FOREIGN KEY (invoice_id) REFERENCES invoices (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_ja_0900_as_cs;

CREATE TABLE daily_reports (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, employee_id BIGINT UNSIGNED NOT NULL, report_date DATE NOT NULL, project_id BIGINT UNSIGNED NULL,
  client_id BIGINT UNSIGNED NULL, parent_report_id BIGINT UNSIGNED NULL, started_at TIME NULL, ended_at TIME NULL, break_minutes SMALLINT UNSIGNED NOT NULL DEFAULT 0,
  work_description VARCHAR(255) NULL, revision INT UNSIGNED NOT NULL DEFAULT 1, deleted_at DATETIME NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id), KEY idx_daily_reports_employee_date (employee_id, report_date), KEY idx_daily_reports_parent (parent_report_id),
  CONSTRAINT fk_daily_reports_employee FOREIGN KEY (employee_id) REFERENCES employees (id), CONSTRAINT fk_daily_reports_project FOREIGN KEY (project_id) REFERENCES projects (id),
  CONSTRAINT fk_daily_reports_client FOREIGN KEY (client_id) REFERENCES clients (id), CONSTRAINT fk_daily_reports_parent FOREIGN KEY (parent_report_id) REFERENCES daily_reports (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_ja_0900_as_cs;

CREATE TABLE schedules (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, employee_id BIGINT UNSIGNED NOT NULL, schedule_date DATE NOT NULL, slot SMALLINT UNSIGNED NOT NULL DEFAULT 1,
  master_item_id BIGINT UNSIGNED NULL, revision INT UNSIGNED NOT NULL DEFAULT 1, deleted_at DATETIME NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id), UNIQUE KEY uq_schedules_employee_day_slot (employee_id, schedule_date, slot),
  CONSTRAINT fk_schedules_employee FOREIGN KEY (employee_id) REFERENCES employees (id), CONSTRAINT fk_schedules_item FOREIGN KEY (master_item_id) REFERENCES master_items (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_ja_0900_as_cs;

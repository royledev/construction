CREATE TABLE employee_qualifications (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  employee_id BIGINT UNSIGNED NOT NULL,
  qualification_name VARCHAR(160) NOT NULL,
  acquired_on DATE NULL,
  renewed_on DATE NULL,
  notes TEXT NULL,
  deleted_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id), KEY idx_employee_qualifications_employee (employee_id),
  CONSTRAINT fk_employee_qualifications_employee FOREIGN KEY (employee_id) REFERENCES employees (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_ja_0900_as_cs;

CREATE TABLE employee_work_items (
  employee_id BIGINT UNSIGNED NOT NULL,
  master_item_id BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (employee_id, master_item_id),
  CONSTRAINT fk_employee_work_items_employee FOREIGN KEY (employee_id) REFERENCES employees (id),
  CONSTRAINT fk_employee_work_items_item FOREIGN KEY (master_item_id) REFERENCES master_items (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_ja_0900_as_cs;

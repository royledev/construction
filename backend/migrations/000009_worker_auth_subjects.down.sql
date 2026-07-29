-- v8 and older cannot represent an external account or schedule. Remove those
-- feature rows before restoring NOT NULL employee columns; employee-owned rows
-- are untouched.
DELETE FROM schedules WHERE subcontractor_worker_id IS NOT NULL;
DELETE FROM accounts WHERE subcontractor_worker_id IS NOT NULL;
ALTER TABLE schedules DROP FOREIGN KEY fk_schedules_subcontractor_worker;
ALTER TABLE schedules DROP FOREIGN KEY fk_schedules_employee;
ALTER TABLE schedules DROP CHECK chk_schedules_subject;
ALTER TABLE schedules DROP INDEX uq_schedules_worker_day_slot;
ALTER TABLE schedules DROP INDEX idx_schedules_subcontractor_worker;
ALTER TABLE schedules DROP COLUMN worker_id;
ALTER TABLE schedules DROP COLUMN worker_type;
ALTER TABLE schedules DROP COLUMN subcontractor_worker_id;
ALTER TABLE schedules MODIFY employee_id BIGINT UNSIGNED NOT NULL;
ALTER TABLE schedules ADD UNIQUE KEY uq_schedules_employee_day_slot (employee_id, schedule_date, slot);
ALTER TABLE schedules ADD CONSTRAINT fk_schedules_employee FOREIGN KEY (employee_id) REFERENCES employees (id);
ALTER TABLE accounts DROP FOREIGN KEY fk_accounts_subcontractor_worker;
ALTER TABLE accounts DROP FOREIGN KEY fk_accounts_employee;
ALTER TABLE accounts DROP CHECK chk_accounts_subject;
ALTER TABLE accounts DROP INDEX idx_accounts_subcontractor_worker;
ALTER TABLE accounts DROP COLUMN subcontractor_worker_id;
ALTER TABLE accounts MODIFY employee_id BIGINT UNSIGNED NOT NULL;
ALTER TABLE accounts ADD CONSTRAINT fk_accounts_employee FOREIGN KEY (employee_id) REFERENCES employees (id);

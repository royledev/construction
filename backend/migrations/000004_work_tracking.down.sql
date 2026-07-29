ALTER TABLE schedules DROP INDEX idx_schedules_date_employee;

ALTER TABLE daily_reports DROP CHECK chk_daily_reports_worker;
ALTER TABLE daily_reports DROP FOREIGN KEY fk_daily_reports_subworker;
ALTER TABLE daily_reports DROP FOREIGN KEY fk_daily_reports_employee;
ALTER TABLE daily_reports DROP INDEX idx_daily_reports_subworker_date;
ALTER TABLE daily_reports DROP COLUMN subcontractor_worker_id;
ALTER TABLE daily_reports MODIFY employee_id BIGINT UNSIGNED NOT NULL;
ALTER TABLE daily_reports ADD CONSTRAINT fk_daily_reports_employee FOREIGN KEY (employee_id) REFERENCES employees (id);

-- T5: daily reports can also be recorded for subcontractor workers.  A report
-- always belongs to exactly one kind of worker; the application enforces that
-- invariant while keeping historical rows soft-deletable.
ALTER TABLE daily_reports DROP FOREIGN KEY fk_daily_reports_employee;
ALTER TABLE daily_reports MODIFY employee_id BIGINT UNSIGNED NULL;
ALTER TABLE daily_reports ADD COLUMN subcontractor_worker_id BIGINT UNSIGNED NULL AFTER employee_id;
ALTER TABLE daily_reports ADD KEY idx_daily_reports_subworker_date (subcontractor_worker_id, report_date);
ALTER TABLE daily_reports ADD CONSTRAINT fk_daily_reports_employee FOREIGN KEY (employee_id) REFERENCES employees (id);
ALTER TABLE daily_reports ADD CONSTRAINT fk_daily_reports_subworker FOREIGN KEY (subcontractor_worker_id) REFERENCES subcontractor_workers (id);
ALTER TABLE daily_reports ADD CONSTRAINT chk_daily_reports_worker CHECK ((employee_id IS NULL) <> (subcontractor_worker_id IS NULL));

ALTER TABLE schedules ADD KEY idx_schedules_date_employee (schedule_date, employee_id, slot);

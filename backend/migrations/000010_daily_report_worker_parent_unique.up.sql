-- 000005's generated unique key predates external workers and is NULL for
-- them. Add a typed owner key so monthly parent initialization is equally safe
-- for employee and subcontractor accounts.
ALTER TABLE daily_reports ADD COLUMN worker_type VARCHAR(16) GENERATED ALWAYS AS (CASE WHEN employee_id IS NULL THEN 'subcontractor' ELSE 'employee' END) STORED;
ALTER TABLE daily_reports ADD COLUMN worker_id BIGINT UNSIGNED GENERATED ALWAYS AS (COALESCE(employee_id, subcontractor_worker_id)) STORED;
ALTER TABLE daily_reports ADD COLUMN auto_parent_worker_type VARCHAR(16) GENERATED ALWAYS AS (CASE WHEN is_month_seeded_parent = 1 THEN worker_type ELSE NULL END) STORED;
ALTER TABLE daily_reports ADD COLUMN auto_parent_worker_id BIGINT UNSIGNED GENERATED ALWAYS AS (CASE WHEN is_month_seeded_parent = 1 THEN worker_id ELSE NULL END) STORED;
ALTER TABLE daily_reports ADD UNIQUE KEY uq_daily_reports_auto_parent_worker (auto_parent_worker_type, auto_parent_worker_id, auto_parent_date);

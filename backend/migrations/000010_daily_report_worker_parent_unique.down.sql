ALTER TABLE daily_reports DROP INDEX uq_daily_reports_auto_parent_worker;
ALTER TABLE daily_reports DROP COLUMN auto_parent_worker_id;
ALTER TABLE daily_reports DROP COLUMN auto_parent_worker_type;
ALTER TABLE daily_reports DROP COLUMN worker_id;
ALTER TABLE daily_reports DROP COLUMN worker_type;

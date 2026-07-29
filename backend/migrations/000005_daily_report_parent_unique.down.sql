ALTER TABLE daily_reports DROP INDEX uq_daily_reports_auto_parent;
ALTER TABLE daily_reports DROP COLUMN auto_parent_date;
ALTER TABLE daily_reports DROP COLUMN is_month_seeded_parent;

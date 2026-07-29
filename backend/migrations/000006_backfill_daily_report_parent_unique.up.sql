-- Forward-only backfill for databases where migration 000005 was already
-- applied before its safe classification rule was added.
UPDATE daily_reports dr
JOIN (
  SELECT employee_id, YEAR(report_date) AS y, MONTH(report_date) AS m
  FROM daily_reports
  WHERE deleted_at IS NULL
  GROUP BY employee_id, YEAR(report_date), MONTH(report_date)
  HAVING SUM(parent_report_id IS NULL) = DAY(LAST_DAY(MIN(report_date)))
     AND COUNT(DISTINCT CASE WHEN parent_report_id IS NULL THEN report_date END) = DAY(LAST_DAY(MIN(report_date)))
     AND SUM(CASE WHEN parent_report_id IS NULL AND project_id IS NULL AND client_id IS NULL AND started_at IS NULL AND ended_at IS NULL AND break_minutes = 0 AND work_description IS NULL AND revision = 1 THEN 1 ELSE 0 END) = DAY(LAST_DAY(MIN(report_date)))
) templates ON templates.employee_id = dr.employee_id AND YEAR(dr.report_date) = templates.y AND MONTH(dr.report_date) = templates.m
SET dr.is_month_seeded_parent = 1
WHERE dr.deleted_at IS NULL AND dr.parent_report_id IS NULL AND dr.project_id IS NULL AND dr.client_id IS NULL AND dr.started_at IS NULL AND dr.ended_at IS NULL AND dr.break_minutes = 0 AND dr.work_description IS NULL AND dr.revision = 1;

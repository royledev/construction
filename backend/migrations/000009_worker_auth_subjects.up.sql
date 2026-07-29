-- An account and a schedule belong to one concrete worker, either an internal
-- employee or an external subcontractor worker.  The discriminator/key pair on
-- schedules keeps MySQL's nullable unique-key semantics from allowing duplicates.
ALTER TABLE accounts DROP FOREIGN KEY fk_accounts_employee;
ALTER TABLE accounts MODIFY employee_id BIGINT UNSIGNED NULL;
ALTER TABLE accounts ADD COLUMN subcontractor_worker_id BIGINT UNSIGNED NULL AFTER employee_id;
ALTER TABLE accounts ADD KEY idx_accounts_subcontractor_worker (subcontractor_worker_id);
ALTER TABLE accounts ADD CONSTRAINT fk_accounts_employee FOREIGN KEY (employee_id) REFERENCES employees (id);
ALTER TABLE accounts ADD CONSTRAINT fk_accounts_subcontractor_worker FOREIGN KEY (subcontractor_worker_id) REFERENCES subcontractor_workers (id);
ALTER TABLE accounts ADD CONSTRAINT chk_accounts_subject CHECK ((employee_id IS NULL) <> (subcontractor_worker_id IS NULL));

ALTER TABLE schedules DROP FOREIGN KEY fk_schedules_employee;
ALTER TABLE schedules DROP INDEX uq_schedules_employee_day_slot;
ALTER TABLE schedules MODIFY employee_id BIGINT UNSIGNED NULL;
ALTER TABLE schedules ADD COLUMN subcontractor_worker_id BIGINT UNSIGNED NULL AFTER employee_id;
ALTER TABLE schedules ADD COLUMN worker_type VARCHAR(16) GENERATED ALWAYS AS (CASE WHEN employee_id IS NULL THEN 'subcontractor' ELSE 'employee' END) STORED;
ALTER TABLE schedules ADD COLUMN worker_id BIGINT UNSIGNED GENERATED ALWAYS AS (COALESCE(employee_id, subcontractor_worker_id)) STORED;
ALTER TABLE schedules ADD KEY idx_schedules_subcontractor_worker (subcontractor_worker_id);
ALTER TABLE schedules ADD UNIQUE KEY uq_schedules_worker_day_slot (worker_type, worker_id, schedule_date, slot);
ALTER TABLE schedules ADD CONSTRAINT fk_schedules_employee FOREIGN KEY (employee_id) REFERENCES employees (id);
ALTER TABLE schedules ADD CONSTRAINT fk_schedules_subcontractor_worker FOREIGN KEY (subcontractor_worker_id) REFERENCES subcontractor_workers (id);
ALTER TABLE schedules ADD CONSTRAINT chk_schedules_subject CHECK ((employee_id IS NULL) <> (subcontractor_worker_id IS NULL));

-- This account is fictional demo data. It is intentionally a worker account,
-- proving that the external-worker auth path is available after upgrade.
INSERT INTO accounts (subcontractor_worker_id, login_id, password_hash, role, seed_marker)
SELECT sw.id, 'takahashi', '$2y$12$7jbHG/IzCyVxPD.A5emlveCU4VMFQ.EZIgAiUImlGXR6ObtoPc6XW', 'user', 'T6-DEMO'
FROM subcontractor_workers sw
JOIN subcontractors sc ON sc.id=sw.subcontractor_id
WHERE sc.subcontractor_code='SUB-0001' AND sw.full_name='高橋 直人' AND sw.notes='架空デモ作業員'
  AND NOT EXISTS (SELECT 1 FROM accounts a WHERE a.login_id='takahashi');

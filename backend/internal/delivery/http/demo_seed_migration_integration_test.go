package httpdelivery

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
)

// TestDemoSeedMigrationLifecycle exercises the SQL files against an isolated
// schema. It covers a clean install, the version-7-to-8 forward upgrade,
// rollback/reapply, fixed-fixture removal, and a conflicting non-demo schedule.
// Set MIGRATION_TEST_ROOT_DSN (a MySQL user allowed to CREATE/DROP DATABASE) to
// run it; ordinary HTTP integration tests need only TEST_DATABASE_DSN.
func TestDemoSeedMigrationLifecycle(t *testing.T) {
	rootDSN := os.Getenv("MIGRATION_TEST_ROOT_DSN")
	if rootDSN == "" {
		t.Skip("MIGRATION_TEST_ROOT_DSN is not set; skipping isolated migration lifecycle test")
	}
	rootConfig, err := mysql.ParseDSN(rootDSN)
	if err != nil {
		t.Fatalf("parse MIGRATION_TEST_ROOT_DSN: %v", err)
	}
	rootConfig.MultiStatements = true
	root, err := sql.Open("mysql", rootConfig.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	if err := root.Ping(); err != nil {
		t.Fatal(err)
	}

	schema := fmt.Sprintf("construction_migration_%x", time.Now().UnixNano())
	if _, err := root.Exec("CREATE DATABASE `" + schema + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = root.Exec("DROP DATABASE IF EXISTS `" + schema + "`") })

	config := *rootConfig
	config.DBName = schema
	config.MultiStatements = true
	db, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	migrationsDir, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	apply := func(version int, direction string) {
		t.Helper()
		path := filepath.Join(migrationsDir, fmt.Sprintf("%06d", version))
		matches, globErr := filepath.Glob(path + "_*." + direction + ".sql")
		if globErr != nil || len(matches) != 1 {
			t.Fatalf("migration %d %s file: %v (%v)", version, direction, matches, globErr)
		}
		sqlText, readErr := os.ReadFile(matches[0])
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, execErr := db.Exec(string(sqlText)); execErr != nil {
			t.Fatalf("migration %d %s (%s): %v", version, direction, matches[0], execErr)
		}
	}
	applyAllUp := func(last int) {
		for version := 1; version <= last; version++ {
			apply(version, "up")
		}
	}
	applyAllDown := func(last int) {
		for version := last; version >= 1; version-- {
			apply(version, "down")
		}
	}
	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := db.QueryRow(query, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	assertDemo := func(expectedAccounts, expectedSchedules int) {
		t.Helper()
		for table, want := range map[string]int{
			"accounts":              expectedAccounts,
			"subcontractor_workers": 1,
			"project_members":       1,
			"daily_reports":         1,
			"estimates":             1,
			"invoices":              1,
		} {
			if got := count("SELECT COUNT(*) FROM " + table + " WHERE seed_marker = 'T6-DEMO'"); got != want {
				t.Fatalf("%s marked demo count = %d, want %d", table, got, want)
			}
		}
		if got := count("SELECT COUNT(*) FROM schedules WHERE seed_marker = 'T6-DEMO'"); got != expectedSchedules {
			t.Fatalf("marked schedule count = %d, want %d", got, expectedSchedules)
		}
		estimateColumn, invoiceColumn := "estimate_number", "invoice_number"
		if count("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name='estimates' AND column_name='estimate_code'") == 1 {
			estimateColumn = "estimate_code"
			invoiceColumn = "invoice_code"
		}
		if got := count("SELECT COUNT(*) FROM estimates WHERE " + estimateColumn + " IN ('EST-T4-001', 'EST-T4-PAGES')"); got != 0 {
			t.Fatalf("residual estimate fixture rows = %d", got)
		}
		if got := count("SELECT COUNT(*) FROM invoices WHERE " + invoiceColumn + " IN ('EST-T4-001', 'EST-T4-PAGES')"); got != 0 {
			t.Fatalf("residual invoice fixture rows = %d", got)
		}
	}

	// Fresh DB -> latest.
	applyAllUp(8)
	assertDemo(1, 2)

	// 000008 down leaves the schema and data in its version-7 shape. A valid
	// business schedule that claims a demo slot must survive the upgrade and
	// must never be marked or deleted by the seed migration.
	apply(8, "down")
	if _, err := db.Exec(`INSERT INTO schedules (employee_id, schedule_date, slot, master_item_id, revision)
		SELECT e.id, '2026-07-25', 1, mi.id, 42 FROM employees e
		JOIN master_items mi ON mi.item_code = 'SCH-001' WHERE e.employee_code = 'EMP-0001'`); err != nil {
		t.Fatal(err)
	}
	apply(8, "up")
	if got := count(`SELECT COUNT(*) FROM schedules s JOIN employees e ON e.id=s.employee_id
		WHERE e.employee_code='EMP-0001' AND s.schedule_date='2026-07-25' AND s.slot=1 AND s.revision=42 AND s.seed_marker IS NULL`); got != 1 {
		t.Fatalf("non-demo schedule was changed during upgrade: %d", got)
	}
	assertDemo(1, 1)

	// Remove only this test business row, then confirm down -> up restores the
	// canonical demo state without duplicates.
	if _, err := db.Exec(`DELETE s FROM schedules s JOIN employees e ON e.id=s.employee_id
		WHERE e.employee_code='EMP-0001' AND s.schedule_date='2026-07-25' AND s.slot=1 AND s.revision=42`); err != nil {
		t.Fatal(err)
	}
	apply(8, "down")
	apply(8, "up")
	assertDemo(1, 2)

	// Every down file must be FK-safe on a fresh/latest schema. Reapplying the
	// full chain proves the all-down state remains installable.
	applyAllDown(8)
	applyAllUp(12)
	assertDemo(2, 2)
	if got := count("SELECT COUNT(*) FROM organization_profiles WHERE id=1 AND company_name='コンストラクションデモ株式会社'"); got != 1 {
		t.Fatalf("organization singleton missing: %d", got)
	}
	if got := count("SELECT COUNT(*) FROM schedule_slots WHERE slot IN (1, 2)"); got != 2 {
		t.Fatalf("seeded schedule slots = %d, want 2", got)
	}
	if got := count("SELECT COUNT(*) FROM accounts WHERE login_id = 'takahashi' AND subcontractor_worker_id IS NOT NULL"); got != 1 {
		t.Fatalf("external demo account count = %d, want 1", got)
	}
	applyAllDown(12)
	applyAllUp(12)
	assertDemo(2, 2)

	// v11 permits the same item code in separate item types.  000012 makes the
	// code the physical primary key, so its down migration must restore the id
	// primary key before restoring these two legacy values.
	apply(12, "down")
	if _, err := db.Exec(`INSERT INTO master_items (item_type, item_code, name) VALUES
		('work', 'LEGACY-SHARED', '旧作業項目'),
		('schedule', 'LEGACY-SHARED', '旧工程項目')`); err != nil {
		t.Fatal(err)
	}
	apply(12, "up")
	apply(12, "down")
	if got := count(`SELECT COUNT(*) FROM master_items WHERE item_code='LEGACY-SHARED' AND item_type IN ('work', 'schedule')`); got != 2 {
		t.Fatalf("legacy cross-type master item codes were not restored: %d", got)
	}
	if _, err := db.Exec(`DELETE FROM master_items WHERE item_code='LEGACY-SHARED' AND item_type IN ('work', 'schedule')`); err != nil {
		t.Fatal(err)
	}
	apply(12, "up")
	assertDemo(2, 2)

}

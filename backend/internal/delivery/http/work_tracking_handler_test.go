package httpdelivery

import (
	"testing"
	"time"
)

func TestReportBreakMinutes(t *testing.T) {
	cases := []struct {
		start, end string
		want       int
		valid      bool
	}{
		{"09:00", "15:00", 0, true},
		{"09:00", "15:01", 45, true},
		{"09:00", "17:01", 60, true},
		{"08:00:00", "17:00:00", 60, true},
		{"", "17:00", 0, true},
		{"17:00", "09:00", 0, false},
		{"nope", "17:00", 0, false},
	}
	for _, tc := range cases {
		got, valid := reportBreakMinutes(tc.start, tc.end)
		if got != tc.want || valid != tc.valid {
			t.Fatalf("%s-%s = %d,%v; want %d,%v", tc.start, tc.end, got, valid, tc.want, tc.valid)
		}
	}
}

func TestMonthBounds(t *testing.T) {
	month, err := time.Parse("2006-01", "2026-02")
	if err != nil {
		t.Fatal(err)
	}
	start, end := monthBounds(month)
	if start != "2026-02-01" || end != "2026-03-01" {
		t.Fatalf("unexpected bounds %s - %s", start, end)
	}
}

func TestStrictScheduleNumbers(t *testing.T) {
	if _, ok := strictPositiveUint(1.5); ok {
		t.Fatal("fractional slot was accepted")
	}
	if got, ok := strictPositiveUint("1"); !ok || got != 1 {
		t.Fatal("canonical string slot was rejected")
	}
	if _, _, ok := strictNullablePositiveUint(float64(0)); ok {
		t.Fatal("zero master item ID was accepted")
	}
	if _, ok := strictRevision(-1.0); ok {
		t.Fatal("negative revision was accepted")
	}
	if got, ok := strictRevision(0.0); !ok || got != 0 {
		t.Fatalf("initial revision = %d,%v", got, ok)
	}
}

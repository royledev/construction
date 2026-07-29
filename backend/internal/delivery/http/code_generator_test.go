package httpdelivery

import (
	"regexp"
	"testing"
)

func TestGenerateBusinessCodeUsesSixDigitPrefixFormat(t *testing.T) {
	pattern := regexp.MustCompile(`^PRJ-[0-9]{6}$`)
	seen := map[string]bool{}
	for range 32 {
		code, err := generateBusinessCode("PRJ")
		if err != nil {
			t.Fatal(err)
		}
		if !pattern.MatchString(code) {
			t.Fatalf("invalid code %q", code)
		}
		seen[code] = true
	}
	if len(seen) < 2 {
		t.Fatal("generator did not vary")
	}
}

func TestImmutableCodeColumns(t *testing.T) {
	if got := immutableCodeColumns("projects"); len(got) != 1 || got[0] != "project_code" {
		t.Fatalf("project code mutability: %#v", got)
	}
	if got := immutableCodeColumns("clients"); len(got) != 1 || got[0] != "client_code" {
		t.Fatalf("client code mutability: %#v", got)
	}
}

func TestBusinessCodeGeneratorIsInjectable(t *testing.T) {
	original := businessCodeGenerator
	t.Cleanup(func() { businessCodeGenerator = original })
	calls := 0
	businessCodeGenerator = func(prefix string) (string, error) { calls++; return prefix + "-000001", nil }
	code, err := businessCodeGenerator("EST")
	if err != nil || code != "EST-000001" || calls != 1 {
		t.Fatalf("injected generator was not used: %q %d", code, calls)
	}
}

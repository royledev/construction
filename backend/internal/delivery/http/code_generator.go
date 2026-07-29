package httpdelivery

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"

	"gorm.io/gorm"
)

// generateBusinessCode deliberately uses crypto/rand rather than predictable
// counters.  The database unique index is the final arbiter; callers retry a
// collision before returning a code to the client.
func generateBusinessCode(prefix string) (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%06d", prefix, n.Int64()), nil
}

var businessCodeGenerator = generateBusinessCode

func uniqueBusinessCode(db *gorm.DB, table, column, prefix string, extra map[string]any) (string, error) {
	for range 32 {
		code, err := businessCodeGenerator(prefix)
		if err != nil {
			return "", err
		}
		query := db.Table(table).Where(column+" = ?", code)
		for key, value := range extra {
			query = query.Where(key+" = ?", value)
		}
		var count int64
		if err := query.Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return code, nil
		}
	}
	return "", errors.New("could not allocate a unique business code")
}

// insertWithGeneratedCode is the collision-safe path: uniqueness is decided by
// INSERT, not a racy preflight COUNT.  Non-code duplicate errors are returned.
func insertWithGeneratedCode(db *gorm.DB, table, column, prefix string, values map[string]any) error {
	for range 32 {
		code, err := businessCodeGenerator(prefix)
		if err != nil {
			return err
		}
		values[column] = code
		err = db.Table(table).Create(values).Error
		if err == nil {
			return nil
		}
		if !isDuplicateKey(err) || !containsCodeKey(err, column) {
			return err
		}
	}
	return errors.New("business code collision retry limit exceeded")
}

func containsCodeKey(err error, column string) bool {
	message := duplicateKeyName(err)
	// ADR-0005 promotes every generated business code to the physical primary
	// key. MySQL reports that collision as e.g. "clients.PRIMARY", rather than
	// repeating the column name. The internal numeric id remains a separately
	// named unique key, so accepting PRIMARY here does not retry that error.
	return message == "" || (len(message) > 0 && (contains(message, column) || contains(message, "code") || contains(message, "PRIMARY")))
}
func contains(s, part string) bool {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return true
		}
	}
	return false
}

func codeSpec(table string, data map[string]any) (column, prefix string, scope map[string]any, ok bool) {
	switch table {
	case "projects":
		return "project_code", "PRJ", nil, true
	case "estimates":
		return "estimate_code", "EST", nil, true
	case "invoices":
		return "invoice_code", "INV", nil, true
	case "employees":
		return "employee_code", "EMP", nil, true
	case "clients":
		return "client_code", "CLI", nil, true
	case "subcontractors":
		return "subcontractor_code", "SUB", nil, true
	case "master_items":
		switch toString(data["item_type"]) {
		case "work":
			return "item_code", "WORK", map[string]any{"item_type": "work"}, true
		case "estimate_invoice":
			return "item_code", "EI", map[string]any{"item_type": "estimate_invoice"}, true
		case "unit_price":
			return "item_code", "UP", map[string]any{"item_type": "unit_price"}, true
		case "schedule":
			return "item_code", "SCH", map[string]any{"item_type": "schedule"}, true
		}
	}
	return "", "", nil, false
}

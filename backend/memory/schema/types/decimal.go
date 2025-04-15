package types

import (
	"database/sql/driver"
	"fmt"

	"entgo.io/ent/dialect"
	"github.com/shopspring/decimal"
)

type Decimal struct {
	decimal.Decimal
}

func (d Decimal) Value() (driver.Value, error) {
	return d.String(), nil
}

func (d *Decimal) Scan(value interface{}) error {
	var err error
	switch v := value.(type) {
	case string:
		d.Decimal, err = decimal.NewFromString(v)
	case []byte:
		d.Decimal, err = decimal.NewFromString(string(v))
	case float64:
		d.Decimal = decimal.NewFromFloat(v)
	case int64:
		d.Decimal = decimal.NewFromInt(v)
	default:
		return fmt.Errorf("unsupported type for Decimal: %T", value)
	}
	return err
}

func DecimalSchemaType() map[string]string {
	return map[string]string{
		dialect.SQLite: "NUMERIC",
	}
}

func DecimalDefault() Decimal {
	return Decimal{Decimal: decimal.NewFromFloat(0)}
}

package models

import (
	"database/sql/driver"
	"errors"
)

type JSONB []byte

func (j JSONB) Value() (driver.Value, error) {
	if len(j) == 0 {
		return []byte("{}"), nil
	}
	return []byte(j), nil
}

func (j *JSONB) Scan(value any) error {
	if value == nil {
		*j = []byte("{}")
		return nil
	}
	switch v := value.(type) {
	case []byte:
		buf := make([]byte, len(v))
		copy(buf, v)
		*j = buf
	case string:
		*j = []byte(v)
	default:
		return errors.New("models.JSONB: unsupported scan type")
	}
	return nil
}

func (j JSONB) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("{}"), nil
	}
	return []byte(j), nil
}

func (j *JSONB) UnmarshalJSON(data []byte) error {
	buf := make([]byte, len(data))
	copy(buf, data)
	*j = buf
	return nil
}

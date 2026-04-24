package repository

import (
	"database/sql"
	"errors"

	"gorm.io/gorm"
)

func translateError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return sql.ErrNoRows
	}
	return err
}

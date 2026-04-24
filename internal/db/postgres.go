package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"bohack_backend_go/internal/config"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func Open(ctx context.Context, cfg config.Config) (*gorm.DB, error) {
	gormDB, err := openByDriver(cfg)
	if err != nil {
		return nil, err
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, err
	}

	switch gormDB.Dialector.Name() {
	case "postgres":
		sqlDB.SetMaxOpenConns(20)
		sqlDB.SetMaxIdleConns(5)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
	case "sqlite":
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	if gormDB.Dialector.Name() == "sqlite" {
		if err := gormDB.WithContext(ctx).Exec("PRAGMA foreign_keys = ON").Error; err != nil {
			return nil, err
		}
	}

	return gormDB, nil
}

func Close(gormDB *gorm.DB) error {
	if gormDB == nil {
		return nil
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func openByDriver(cfg config.Config) (*gorm.DB, error) {
	gconf := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
		NamingStrategy: schema.NamingStrategy{
			SingularTable: false,
		},
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	switch strings.ToLower(strings.TrimSpace(cfg.DBDriver)) {
	case "sqlite", "sqlite3":
		dsn := cfg.DatabaseURL
		if !strings.Contains(dsn, "_pragma=") {
			separator := "?"
			if strings.Contains(dsn, "?") {
				separator = "&"
			}
			dsn = dsn + separator + "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
		}
		return gorm.Open(sqlite.Open(dsn), gconf)
	case "", "postgres", "postgresql", "pg":
		return gorm.Open(postgres.Open(cfg.DatabaseURL), gconf)
	default:
		return nil, fmt.Errorf("unsupported DB_DRIVER: %s", cfg.DBDriver)
	}
}

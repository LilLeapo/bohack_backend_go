package db

import (
	"context"
	"time"

	"bohack_backend_go/internal/config"
	"bohack_backend_go/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func EnsureSchema(ctx context.Context, gormDB *gorm.DB, cfg config.Config) error {
	tx := gormDB.WithContext(ctx)

	switch tx.Dialector.Name() {
	case "postgres":
		if err := applyPostgresPreMigrate(tx); err != nil {
			return err
		}
	}

	if err := tx.AutoMigrate(
		&models.User{},
		&models.VerificationCode{},
		&models.Event{},
		&models.Registration{},
		&models.RegistrationAttachment{},
		&models.AttendanceConfirmation{},
	); err != nil {
		return err
	}

	switch tx.Dialector.Name() {
	case "postgres":
		if err := applyPostgresPostMigrate(tx); err != nil {
			return err
		}
	case "sqlite":
		if err := applySQLitePostMigrate(tx); err != nil {
			return err
		}
	}

	now := time.Now().UTC()
	defaultEvent := models.Event{
		Slug:      cfg.DefaultEventSlug,
		Title:     cfg.DefaultEventTitle,
		Status:    "published",
		IsCurrent: false,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "slug"}},
		DoNothing: true,
	}).Create(&defaultEvent).Error; err != nil {
		return err
	}

	var currentCount int64
	if err := tx.Model(&models.Event{}).Where("is_current = ?", true).Count(&currentCount).Error; err != nil {
		return err
	}
	if currentCount == 0 {
		if err := tx.Model(&models.Event{}).
			Where("slug = ?", cfg.DefaultEventSlug).
			Updates(map[string]any{
				"is_current": true,
				"updated_at": time.Now().UTC(),
			}).Error; err != nil {
			return err
		}
	}

	return nil
}

func applyPostgresPreMigrate(tx *gorm.DB) error {
	stmts := []string{
		// Older schemas created implicit UNIQUE constraints with *_key names. GORM v1.31
		// tries to drop generated uni_* constraints for unique columns before recreating
		// unique indexes, which crashes when those legacy names don't match.
		`ALTER TABLE IF EXISTS users DROP CONSTRAINT IF EXISTS users_username_key`,
		`ALTER TABLE IF EXISTS users DROP CONSTRAINT IF EXISTS users_email_key`,
		`ALTER TABLE IF EXISTS events DROP CONSTRAINT IF EXISTS events_slug_key`,
		`ALTER TABLE IF EXISTS verification_codes DROP CONSTRAINT IF EXISTS verification_codes_email_code_type_key`,
		`ALTER TABLE IF EXISTS event_registrations DROP CONSTRAINT IF EXISTS event_registrations_event_id_user_id_key`,
	}
	for _, stmt := range stmts {
		if err := tx.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}

func applyPostgresPostMigrate(tx *gorm.DB) error {
	stmts := []string{
		`DO $$ BEGIN
			IF EXISTS (
				SELECT 1
				FROM information_schema.columns
				WHERE table_schema = current_schema()
					AND table_name = 'event_registrations'
					AND column_name = 'extra'
					AND udt_name <> 'jsonb'
			) THEN
				ALTER TABLE event_registrations ALTER COLUMN extra DROP DEFAULT;
				ALTER TABLE event_registrations ALTER COLUMN extra TYPE jsonb USING extra::jsonb;
			END IF;
			ALTER TABLE event_registrations ALTER COLUMN extra SET DEFAULT '{}'::jsonb;
		END $$;`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_events_single_current ON events (is_current) WHERE is_current`,
		`DO $$ BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint WHERE conname = 'event_registrations_event_id_fkey'
			) THEN
				ALTER TABLE event_registrations
					ADD CONSTRAINT event_registrations_event_id_fkey
					FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE RESTRICT;
			END IF;
		END $$;`,
		`DO $$ BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint WHERE conname = 'event_registrations_user_id_fkey'
			) THEN
				ALTER TABLE event_registrations
					ADD CONSTRAINT event_registrations_user_id_fkey
					FOREIGN KEY (user_id) REFERENCES users(uid) ON DELETE RESTRICT;
			END IF;
		END $$;`,
		`DO $$ BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint WHERE conname = 'event_registrations_reviewed_by_fkey'
			) THEN
				ALTER TABLE event_registrations
					ADD CONSTRAINT event_registrations_reviewed_by_fkey
					FOREIGN KEY (reviewed_by) REFERENCES users(uid) ON DELETE SET NULL;
			END IF;
		END $$;`,
		`DO $$ BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint WHERE conname = 'event_registration_attachments_registration_id_fkey'
			) THEN
				ALTER TABLE event_registration_attachments
					ADD CONSTRAINT event_registration_attachments_registration_id_fkey
					FOREIGN KEY (registration_id) REFERENCES event_registrations(id) ON DELETE CASCADE;
			END IF;
		END $$;`,
		`DO $$ BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint WHERE conname = 'attendance_confirmations_registration_id_fkey'
			) THEN
				ALTER TABLE attendance_confirmations
					ADD CONSTRAINT attendance_confirmations_registration_id_fkey
					FOREIGN KEY (registration_id) REFERENCES event_registrations(id) ON DELETE CASCADE;
			END IF;
		END $$;`,
		`DO $$ BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint WHERE conname = 'attendance_confirmations_user_id_fkey'
			) THEN
				ALTER TABLE attendance_confirmations
					ADD CONSTRAINT attendance_confirmations_user_id_fkey
					FOREIGN KEY (user_id) REFERENCES users(uid) ON DELETE CASCADE;
			END IF;
		END $$;`,
		`DO $$ BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint WHERE conname = 'events_status_check'
			) THEN
				ALTER TABLE events
					ADD CONSTRAINT events_status_check
					CHECK (status IN ('draft', 'published', 'archived'));
			END IF;
		END $$;`,
		`DO $$ BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint WHERE conname = 'event_registrations_status_check'
			) THEN
				ALTER TABLE event_registrations
					ADD CONSTRAINT event_registrations_status_check
					CHECK (status IN ('draft', 'submitted', 'under_review', 'approved', 'rejected', 'cancelled'));
			END IF;
		END $$;`,
		`DO $$ BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint WHERE conname = 'attendance_confirmations_status_check'
			) THEN
				ALTER TABLE attendance_confirmations
					ADD CONSTRAINT attendance_confirmations_status_check
					CHECK (status IN ('pending', 'confirmed', 'declined'));
			END IF;
		END $$;`,
	}
	for _, stmt := range stmts {
		if err := tx.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}

func applySQLitePostMigrate(tx *gorm.DB) error {
	stmts := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_events_single_current ON events (is_current) WHERE is_current = 1`,
	}
	for _, stmt := range stmts {
		if err := tx.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}

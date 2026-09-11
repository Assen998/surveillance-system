package database

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sirupsen/logrus"
	"github.com/yourorg/surveillance-system/internal/config"
	"github.com/yourorg/surveillance-system/internal/models"
)

var DB *gorm.DB

var FirstRun bool

func Init(cfg *config.Config) error {
	var dialector gorm.Dialector

	switch cfg.Database.Type {
	case "sqlite":

		dbPath := cfg.Database.SQLite.Path
		if _, statErr := os.Stat(dbPath); statErr != nil {
			FirstRun = true
		}

		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create database directory: %w", err)
		}

		dialector = sqlite.Open(dbPath + "?_foreign_keys=on&_journal_mode=WAL&_synchronous=NORMAL")
	case "postgres":
		_ = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			cfg.Database.Postgres.Host,
			cfg.Database.Postgres.Port,
			cfg.Database.Postgres.User,
			cfg.Database.Postgres.Password,
			cfg.Database.Postgres.DBName,
			cfg.Database.Postgres.SSLMode,
		)

		return fmt.Errorf("PostgreSQL support not implemented yet")
	default:
		return fmt.Errorf("unsupported database type: %s", cfg.Database.Type)
	}

	var err error
	DB, err = gorm.Open(dialector, &gorm.Config{
		Logger:      logger.Default.LogMode(logLevel(cfg.Logging.Level)),
		PrepareStmt: true,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying connection: %w", err)
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(10)

	if err := autoMigrate(); err != nil {
		return fmt.Errorf("database migration failed: %w", err)
	}

	if FirstRun {
		logrus.Info("fresh install detected: skipping default admin creation, please create an admin account on the web first-time setup page")
	} else if err := initDefaultData(); err != nil {
		logrus.Warnf("failed to initialize default data: %v", err)
	}

	logrus.Info("database initialization completed")
	return nil
}

func logLevel(level string) logger.LogLevel {
	switch level {
	case "debug":
		return logger.Info
	case "info":
		return logger.Warn
	case "warn":
		return logger.Warn
	case "error":
		return logger.Error
	default:
		return logger.Warn
	}
}

func autoMigrate() error {
	return DB.AutoMigrate(
		&models.Camera{},
		&models.Recording{},
		&models.Snapshot{},
		&models.Alert{},
		&models.User{},
		&models.CameraPermission{},
		&models.SystemConfig{},
	)
}

func initDefaultData() error {

	const defaultAdminHash = "$2a$10$MY62Xh/mqv2QCIb.NI19WOS0nSLxStwPtC/NrmhraUn3zPBBPxmOq"

	const legacyPlaceholderHash = "$2a$10$XQxQxQxQxQxQxQxQxQxQxO"

	var count int64
	DB.Model(&models.User{}).Where("role = ?", models.UserRoleAdmin).Count(&count)
	if count == 0 {
		admin := &models.User{
			Username: "admin",
			Password: defaultAdminHash,
			Email:    "admin@localhost",
			Role:     models.UserRoleAdmin,
			Status:   "active",
		}
		if err := DB.Create(admin).Error; err != nil {
			return err
		}
		logrus.Info("created default admin user: admin/admin123 (please change the password after logging in as soon as possible)")
		return nil
	}

	res := DB.Model(&models.User{}).
		Where("username = ? AND password = ?", "admin", legacyPlaceholderHash).
		Update("password", defaultAdminHash)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		logrus.Info("legacy placeholder password detected, reset to default password: admin/admin123 (please change it as soon as possible)")
	}
	return nil
}

func GetDB() *gorm.DB {
	return DB
}

func Close() error {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}

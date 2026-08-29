package database

import (
	"fmt"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/yourorg/surveillance-system/internal/config"
	"github.com/yourorg/surveillance-system/internal/models"
	"github.com/sirupsen/logrus"
)

var DB *gorm.DB

func Init(cfg *config.Config) error {
	var dialector gorm.Dialector

	switch cfg.Database.Type {
	case "sqlite":
		// 确保目录存在
		dbPath := cfg.Database.SQLite.Path
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建数据库目录失败: %w", err)
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
		// 需要导入 gorm.io/driver/postgres
		// dialector = postgres.Open(dsn)
		return fmt.Errorf("PostgreSQL 支持待实现")
	default:
		return fmt.Errorf("不支持的数据库类型: %s", cfg.Database.Type)
	}

	var err error
	DB, err = gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logLevel(cfg.Logging.Level)),
		PrepareStmt: true,
	})
	if err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}

	// 连接池配置
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("获取底层连接失败: %w", err)
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(10)

	// 自动迁移
	if err := autoMigrate(); err != nil {
		return fmt.Errorf("数据库迁移失败: %w", err)
	}

	// 初始化默认数据
	if err := initDefaultData(); err != nil {
		logrus.Warnf("初始化默认数据失败: %v", err)
	}

	logrus.Info("数据库初始化完成")
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
	// bcrypt hash of "admin123"（真实哈希，可直接用于登录校验）
	const defaultAdminHash = "$2a$10$MY62Xh/mqv2QCIb.NI19WOS0nSLxStwPtC/NrmhraUn3zPBBPxmOq"
	// 旧版本写入的占位哈希（无法匹配任何密码），检测到则重置为默认密码
	const legacyPlaceholderHash = "$2a$10$XQxQxQxQxQxQxQxQxQxQxO"

	// 检查是否已有管理员用户
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
		logrus.Info("创建默认管理员用户: admin/admin123 (请登录后尽快修改密码)")
		return nil
	}

	// 升级路径：将旧占位密码修复为可用的默认密码
	res := DB.Model(&models.User{}).
		Where("username = ? AND password = ?", "admin", legacyPlaceholderHash).
		Update("password", defaultAdminHash)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		logrus.Info("检测到旧版占位密码，已重置为默认密码: admin/admin123 (请尽快修改)")
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
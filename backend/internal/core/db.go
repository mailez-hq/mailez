package core

import (
	glebarezsqlite "github.com/glebarez/sqlite"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// OpenDB opens the configured database: sqlite (pure-Go, local dev) or
// mysql (production). The DSN formats are the usual ones, e.g.
//
//	sqlite: /data/mailez.db
//	mysql:  user:pass@tcp(host:3306)/mailez?charset=utf8mb4&parseTime=True&loc=Local
//
// SingularTable keeps table names aligned with the model names on both
// engines.
func OpenDB(driver, dsn, logLevel string) (*gorm.DB, error) {
	level := logger.Warn
	if logLevel == "debug" || logLevel == "trace" {
		level = logger.Info
	}
	var dialector gorm.Dialector
	switch driver {
	case "mysql":
		dialector = gormmysql.Open(dsn)
	default:
		dialector = glebarezsqlite.Open(dsn)
	}
	return gorm.Open(dialector, &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
		Logger:         logger.Default.LogMode(level),
	})
}

package database

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/kpkipper/journey-service/config"
	"github.com/kpkipper/journey-service/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var DB *gorm.DB

func NewConnection(conf config.Postgres) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		conf.Host, conf.Port, conf.User, conf.Password, conf.DBName, conf.SSLMode)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		TranslateError: true,
		// Logs go to stderr so admin CLI commands keep stdout clean.
		Logger: gormlogger.New(log.New(os.Stderr, "", log.LstdFlags), gormlogger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  gormlogger.Warn,
			IgnoreRecordNotFoundError: true,
		}),
	})

	if err != nil {
		return db, err
	}
	return db, nil
}

// Models lists every model that must be migrated. Add new models here.
func Models() []any {
	return []any{
		&models.Journey{},
		&models.ItineraryDay{},
		&models.ActivityPlan{},
		&models.User{},
		&models.Session{},
		&models.LoginToken{},
		&models.JourneyMember{},
		&models.InviteLink{},
		&models.Follow{},
		&models.Block{},
		&models.Report{},
		&models.Moment{},
	}
}

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(Models()...)
}

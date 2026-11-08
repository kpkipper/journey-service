package database

import (
	"fmt"

	"github.com/kpkipper/journey-service/config"
	"github.com/kpkipper/journey-service/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func NewConnection(conf config.Postgres) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		conf.Host, conf.Port, conf.User, conf.Password, conf.DBName, conf.SSLMode)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{TranslateError: true})

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
	}
}

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(Models()...)
}

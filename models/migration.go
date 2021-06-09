package models

import "github.com/jinzhu/gorm"

func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&Route{}, &Certificate{}).Error; err != nil {
		return err
	}
	return nil
}

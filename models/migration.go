package models

import "github.com/jinzhu/gorm"

func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&Route{}, &Certificate{}, &UserData{}).Error; err != nil {
		return err
	}
	db.Model(&UserData{}).RemoveIndex("uix_user_data_email")
	return nil
}

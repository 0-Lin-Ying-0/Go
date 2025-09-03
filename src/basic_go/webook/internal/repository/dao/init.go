package dao

import "gorm.io/gorm"

func InitTables(db *gorm.DB) error {
	// 严格来说，这不是一个好的实践
	return db.AutoMigrate(&User{})
}

package main

import (
	"fmt"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=UTC",
		os.Getenv("DB_HOST"), os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"), os.Getenv("DB_PORT"), os.Getenv("DB_SSLMODE"))
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil { fmt.Println("open:", err); os.Exit(1) }
	type Row struct {
		ID        string `gorm:"column:id"`
		Name      string `gorm:"column:name"`
		Scope     string `gorm:"column:scope"`
		BuiltinKey string `gorm:"column:builtin_key"`
	}
	var rows []Row
	db.Table("fleet_categories").Select("id, name, scope, builtin_key").Where("scope='maintain' AND deleted_at IS NULL").Order("created_at").Find(&rows)
	for _, r := range rows {
		fmt.Printf("name=%q builtin_key=%q\n", r.Name, r.BuiltinKey)
	}
}

package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	dsn := "host=db.kunsjmpkvfjjzcotdgrg.supabase.co port=5432 user=postgres password=7aPBAINUbuMmTD dbname=postgres sslmode=require"
	db, err := sql.Open("postgres", dsn)
	if err != nil { fmt.Println("open:", err); os.Exit(1) }
	defer db.Close()
	rows, err := db.Query(`SELECT scope, group_key, name FROM fleet_group_aliases ORDER BY scope, group_key`)
	if err != nil { fmt.Println("query:", err); os.Exit(1) }
	defer rows.Close()
	for rows.Next() {
		var s, k, n string
		rows.Scan(&s, &k, &n)
		fmt.Printf("scope=%s group_key=%s name=%s\n", s, k, n)
	}
}

package main

import (
	"flag"
	"log"
)

func main() {

	filePath := flag.String("file", "/opt/db/db.sqlite3", "Path to sqlite3 file")
	shouldLog := flag.Bool("log", false, "Log events to vni_allocs_log")
	flag.Parse()

	if err := StartServer(filePath, *shouldLog); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}

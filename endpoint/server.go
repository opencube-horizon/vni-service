package main

import (
	"database/sql"
	"github.com/mattn/go-sqlite3"
	"log"
	"net/http"
)

var vniMin = 100
var vniMax = 65535
var shouldLog bool
var DBFilePath *string

func StartServer(filePath *string, _shouldLog bool) error {
	shouldLog = _shouldLog
	DBFilePath = filePath
	sql.Register("sqlite3_with_extensions", &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			return conn.CreateModule("generate_series", &seriesModule{})
		},
	})

	db, err := open(DBFilePath)
	if err != nil {
		log.Fatalf("Error opening db: %v\n", err.Error())
		return err
	}

	err = Init(db)
	if err != nil {
		log.Fatalf("Error initializing DB: %s\n", err)
		return err
	}
	err = db.Close()
	if err != nil {
		log.Fatalf("Error closing DB: %s\n", err)
		return err
	}

	http.HandleFunc("/version", cVersion)
	http.HandleFunc("/sync", cSync)
	http.HandleFunc("/finalize", cFinalize)

	log.Printf("Starting server (v1.0) at port 8842 (logging: %v)\n", shouldLog)
	err = http.ListenAndServe(":8842", nil)
	if err != nil {
		log.Printf("Error while starting server: %v\n",
			err)
		return err
	}
	return nil
}

package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/mattn/go-sqlite3"
)

var vniMin int
var vniMax int
var shouldLog bool
var DBFilePath *string

func StartServer(filePath *string, _shouldLog bool, port int, _vniMin int, _vniMax int) error {
	shouldLog = _shouldLog
	vniMin = _vniMin
	vniMax = _vniMax
	DBFilePath = filePath
	sql.Register("sqlite3_with_extensions", &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			return conn.CreateModule("generate_series", &seriesModule{})
		},
	})

	db, err := open(DBFilePath)
	if err != nil {
		slog.Error("Error opening db", "err", err.Error())
		return err
	}

	err = Init(db)
	if err != nil {
		slog.Error("Error initializing DB", "err", err)
		return err
	}
	err = db.Close()
	if err != nil {
		slog.Error("Error closing DB", "err", err)
		return err
	}

	http.HandleFunc("/version", cVersion)
	http.HandleFunc("/sync", cSync)
	http.HandleFunc("/finalize", cFinalize)

	slog.Info(fmt.Sprintf("Starting server at port %d, assigning VNIs in range %d-%d", port, vniMin, vniMax))
	err = http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	if err != nil {
		slog.Error("Error while starting server", "err", err)
		return err
	}
	return nil
}

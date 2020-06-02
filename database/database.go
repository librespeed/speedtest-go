package database

import (
	"github.com/librespeed/speedtest/config"
	"github.com/librespeed/speedtest/database/bolt"
	"github.com/librespeed/speedtest/database/mysql"
	"github.com/librespeed/speedtest/database/postgresql"
	"github.com/librespeed/speedtest/database/schema"
)

var (
	DB DataAccess
)

type DataAccess interface {
	Insert(*schema.TelemetryData) error
	FetchByUUID(string) (*schema.TelemetryData, error)
	FetchLast100() ([]schema.TelemetryData, error)
}

func SetDBInfo(conf *config.Config) {
	switch conf.DatabaseType {
	case "postgresql":
		DB = postgresql.Open(conf.DatabaseHostname, conf.DatabaseUsername, conf.DatabasePassword, conf.DatabaseName)
	case "mysql":
		DB = mysql.Open(conf.DatabaseHostname, conf.DatabaseUsername, conf.DatabasePassword, conf.DatabaseName)
	case "bolt":
		DB = bolt.Open(conf.DatabaseFile)
	}
}

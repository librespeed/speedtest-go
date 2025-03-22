package database

import (
	"github.com/librespeed/speedtest-go/config"
	"github.com/librespeed/speedtest-go/database/bolt"
	"github.com/librespeed/speedtest-go/database/memory"
	"github.com/librespeed/speedtest-go/database/mysql"
	"github.com/librespeed/speedtest-go/database/none"
	"github.com/librespeed/speedtest-go/database/postgresql"
	"github.com/librespeed/speedtest-go/database/schema"

	log "github.com/sirupsen/logrus"
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
	case "memory":
		DB = memory.Open("")
	case "none":
		DB = none.Open("")
	default:
		log.Fatalf("Unsupported database type: %s", conf.DatabaseType)
	}
}

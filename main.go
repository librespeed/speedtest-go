package main

import (
	"github.com/librespeed/speedtest/config"
	"github.com/librespeed/speedtest/database"
	"github.com/librespeed/speedtest/web"

	log "github.com/sirupsen/logrus"
)

func main() {
	conf := config.Load()

	database.SetDBInfo(&conf)
	log.Fatal(web.ListenAndServe(&conf))
}

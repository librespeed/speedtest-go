package main

import (
	"flag"
	_ "time/tzdata"

	"github.com/librespeed/speedtest-go/config"
	"github.com/librespeed/speedtest-go/database"
	"github.com/librespeed/speedtest-go/results"
	"github.com/librespeed/speedtest-go/web"

	_ "github.com/breml/rootcerts"
	log "github.com/sirupsen/logrus"
)

var (
	optConfig = flag.String("c", "", "config file to be used, defaults to settings.toml in the same directory")
)

func main() {
	flag.Parse()
	conf := config.Load(*optConfig)
	web.SetServerLocation(&conf)
	results.Initialize(&conf)
	database.SetDBInfo(&conf)
	log.Fatal(web.ListenAndServe(&conf))
}

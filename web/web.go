package web

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/render"
	log "github.com/sirupsen/logrus"

	"github.com/librespeed/speedtest/config"
	"github.com/librespeed/speedtest/results"
)

const (
	// chunk size is 1 mib
	chunkSize = 1048576
)

var (
	// generate random data for download test on start to minimize runtime overhead
	randomData = getRandomData(chunkSize)
)

func ListenAndServe(conf *config.Config) error {
	r := chi.NewMux()
	r.Use(middleware.RealIP)

	cs := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "OPTIONS", "HEAD"},
		AllowedHeaders: []string{"*"},
	})

	r.Use(cs.Handler)
	r.Use(middleware.NoCache)
	r.Use(middleware.Logger)

	log.Infof("Starting backend server on %s", net.JoinHostPort(conf.BindAddress, conf.Port))
	r.Get("/*", pages)
	r.Head("/*", pages)
	r.HandleFunc("/empty", empty)
	r.HandleFunc("/backend/empty", empty)
	r.Get("/garbage", garbage)
	r.Head("/garbage", garbage)
	r.Get("/backend/garbage", garbage)
	r.Head("/backend/garbage", garbage)
	r.Get("/getIP", getIP)
	r.Head("/getIP", getIP)
	r.Get("/backend/getIP", getIP)
	r.Head("/backend/getIP", getIP)
	r.Get("/results", results.DrawPNG)
	r.Get("/results/", results.DrawPNG)
	r.Get("/backend/results", results.DrawPNG)
	r.Get("/backend/results/", results.DrawPNG)
	r.Post("/results/telemetry", results.Record)
	r.Post("/backend/results/telemetry", results.Record)
	r.HandleFunc("/stats", results.Stats)
	r.HandleFunc("/backend/stats", results.Stats)

	// PHP frontend default values compatibility
	r.HandleFunc("/empty.php", empty)
	r.HandleFunc("/backend/empty.php", empty)
	r.Get("/garbage.php", garbage)
	r.Head("/garbage.php", garbage)
	r.Get("/backend/garbage.php", garbage)
	r.Head("/backend/garbage.php", garbage)
	r.Get("/getIP.php", getIP)
	r.Head("/getIP.php", getIP)
	r.Get("/backend/getIP.php", getIP)
	r.Head("/backend/getIP.php", getIP)
	r.Post("/results/telemetry.php", results.Record)
	r.Post("/backend/results/telemetry.php", results.Record)
	r.HandleFunc("/stats.php", results.Stats)
	r.HandleFunc("/backend/stats.php", results.Stats)

	return http.ListenAndServe(net.JoinHostPort(conf.BindAddress, conf.Port), r)
}

func pages(w http.ResponseWriter, r *http.Request) {
	if r.RequestURI == "/" {
		r.RequestURI = "/index.html"
	}

	conf := config.LoadedConfig()

	uri := strings.Split(r.RequestURI, "?")[0]
	if strings.HasSuffix(uri, ".html") || strings.HasSuffix(uri, ".js") {
		http.FileServer(http.Dir(conf.AssetsPath)).ServeHTTP(w, r)
	} else {
		w.WriteHeader(http.StatusForbidden)
	}
}

func empty(w http.ResponseWriter, r *http.Request) {
	io.Copy(ioutil.Discard, r.Body)
	r.Body.Close()

	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
}

func garbage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Description", "File Transfer")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=random.dat")
	w.Header().Set("Content-Transfer-Encoding", "binary")

	// chunk size set to 4 by default
	chunks := 4

	ckSize := r.FormValue("ckSize")
	if ckSize != "" {
		i, err := strconv.ParseInt(ckSize, 10, 64)
		if err != nil {
			log.Errorf("Invalid chunk size: %s", ckSize)
			log.Warn("Will use default value %d", chunks)
		} else {
			// limit max chunk size to 1024
			if i > 1024 {
				chunks = 1024
			} else {
				chunks = int(i)
			}
		}
	}

	for i := 0; i < chunks; i++ {
		if _, err := w.Write(randomData); err != nil {
			log.Errorf("Error writing back to client at chunk number %d: %s", i, err)
			break
		}
	}
}

func getIP(w http.ResponseWriter, r *http.Request) {
	var ret results.Result

	clientIP := r.RemoteAddr
	clientIP = strings.ReplaceAll(clientIP, "::ffff:", "")

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		clientIP = ip
	}

	isSpecialIP := true
	switch {
	case clientIP == "::1":
		ret.ProcessedString = clientIP + " - localhost IPv6 access"
	case strings.HasPrefix(clientIP, "fe80:"):
		ret.ProcessedString = clientIP + " - link-local IPv6 access"
	case strings.HasPrefix(clientIP, "127."):
		ret.ProcessedString = clientIP + " - localhost IPv4 access"
	case strings.HasPrefix(clientIP, "10."):
		ret.ProcessedString = clientIP + " - private IPv4 access"
	case regexp.MustCompile(`^172\.(1[6-9]|2\d|3[01])\.`).MatchString(clientIP):
		ret.ProcessedString = clientIP + " - private IPv4 access"
	case strings.HasPrefix(clientIP, "192.168"):
		ret.ProcessedString = clientIP + " - private IPv4 access"
	case strings.HasPrefix(clientIP, "169.254"):
		ret.ProcessedString = clientIP + " - link-local IPv4 access"
	case regexp.MustCompile(`^100\.([6-9][0-9]|1[0-2][0-7])\.`).MatchString(clientIP):
		ret.ProcessedString = clientIP + " - CGNAT IPv4 access"
	default:
		isSpecialIP = false
	}

	if isSpecialIP {
		b, _ := json.Marshal(&ret)
		if _, err := w.Write(b); err != nil {
			log.Errorf("Error writing to client: %s", err)
		}
		return
	}

	getISPInfo := r.FormValue("isp") == "true"
	distanceUnit := r.FormValue("distance")

	ret.ProcessedString = clientIP

	if getISPInfo {
		ispInfo := getIPInfo(clientIP)
		ret.RawISPInfo = ispInfo

		removeRegexp := regexp.MustCompile(`AS\d+\s`)
		isp := removeRegexp.ReplaceAllString(ispInfo.Organization, "")

		if isp == "" {
			isp = "Unknown ISP"
		}

		if ispInfo.Country != "" {
			isp += ", " + ispInfo.Country
		}

		if ispInfo.Location != "" {
			isp += " (" + calculateDistance(ispInfo.Location, distanceUnit) + ")"
		}

		ret.ProcessedString += " - " + isp
	}

	render.JSON(w, r, ret)
}

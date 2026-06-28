package web

import (
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/render"
	"github.com/pires/go-proxyproto"
	log "github.com/sirupsen/logrus"

	"github.com/librespeed/speedtest-go/config"
	"github.com/librespeed/speedtest-go/results"
)

const (
	// chunk size is 1 mib
	chunkSize = 1048576
)

//go:embed assets
var defaultAssets embed.FS

var (
	// generate random data for download test on start to minimize runtime overhead
	randomData = getRandomData(chunkSize)
)

func ListenAndServe(conf *config.Config) error {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.GetHead)

	cs := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "OPTIONS", "HEAD"},
		AllowedHeaders: []string{"*"},
	})

	r.Use(cs.Handler)
	r.Use(middleware.NoCache)
	r.Use(middleware.Recoverer)

	var assetFS http.FileSystem
	if fi, err := os.Stat(conf.AssetsPath); os.IsNotExist(err) || !fi.IsDir() {
		log.Warnf("Configured asset path %s does not exist or is not a directory, using default assets", conf.AssetsPath)
		sub, err := fs.Sub(defaultAssets, "assets")
		if err != nil {
			log.Fatalf("Failed when processing default assets: %s", err)
		}
		assetFS = http.FS(sub)
	} else {
		assetFS = justFilesFilesystem{fs: http.Dir(conf.AssetsPath), readDirBatchSize: 2}
	}

	r.Get(conf.BaseURL+"/*", pages(assetFS, conf.BaseURL))
	r.HandleFunc(conf.BaseURL+"/empty", empty)
	r.HandleFunc(conf.BaseURL+"/backend/empty", empty)
	r.Get(conf.BaseURL+"/garbage", garbage)
	r.Get(conf.BaseURL+"/backend/garbage", garbage)
	r.Get(conf.BaseURL+"/getIP", getIP)
	r.Get(conf.BaseURL+"/backend/getIP", getIP)
	r.Get(conf.BaseURL+"/results/view", results.ViewPage)
	r.Get(conf.BaseURL+"/results", results.DrawPNG)
	r.Get(conf.BaseURL+"/results/", results.DrawPNG)
	r.Get(conf.BaseURL+"/backend/results", results.DrawPNG)
	r.Get(conf.BaseURL+"/backend/results/", results.DrawPNG)
	r.Post(conf.BaseURL+"/results/telemetry", results.Record)
	r.Post(conf.BaseURL+"/backend/results/telemetry", results.Record)
	r.HandleFunc(conf.BaseURL+"/stats", results.Stats)
	r.HandleFunc(conf.BaseURL+"/backend/stats", results.Stats)
	r.Get(conf.BaseURL+"/results/json", results.JSONResult)
	r.Get(conf.BaseURL+"/backend/results/json", results.JSONResult)

	// PHP frontend default values compatibility
	r.HandleFunc(conf.BaseURL+"/empty.php", empty)
	r.HandleFunc(conf.BaseURL+"/backend/empty.php", empty)
	r.Get(conf.BaseURL+"/garbage.php", garbage)
	r.Get(conf.BaseURL+"/backend/garbage.php", garbage)
	r.Get(conf.BaseURL+"/getIP.php", getIP)
	r.Get(conf.BaseURL+"/backend/getIP.php", getIP)
	r.Post(conf.BaseURL+"/results/telemetry.php", results.Record)
	r.Post(conf.BaseURL+"/backend/results/telemetry.php", results.Record)
	r.HandleFunc(conf.BaseURL+"/stats.php", results.Stats)
	r.HandleFunc(conf.BaseURL+"/backend/stats.php", results.Stats)
	r.Get(conf.BaseURL+"/results/json.php", results.JSONResult)
	r.Get(conf.BaseURL+"/backend/results/json.php", results.JSONResult)

	go listenProxyProtocol(conf, r)

	return startListener(conf, r)
}

func listenProxyProtocol(conf *config.Config, r *chi.Mux) {
	if conf.ProxyProtocolPort != "0" {
		addr := net.JoinHostPort(conf.BindAddress, conf.ProxyProtocolPort)
		l, err := net.Listen("tcp", addr)
		if err != nil {
			log.Fatalf("Cannot listen on proxy protocol port %s: %s", conf.ProxyProtocolPort, err)
		}

		pl := &proxyproto.Listener{Listener: l}
		defer pl.Close()

		log.Infof("Starting proxy protocol listener on %s", addr)
		log.Fatal(http.Serve(pl, r))
	}
}

func pages(fs http.FileSystem, BaseURL string) http.HandlerFunc {
	var removeBaseURL *regexp.Regexp
	if BaseURL != "" {
		removeBaseURL = regexp.MustCompile("^" + BaseURL + "/")
	}
	fn := func(w http.ResponseWriter, r *http.Request) {
		if BaseURL != "" {
			r.URL.Path = removeBaseURL.ReplaceAllString(r.URL.Path, "/")
		}
		if r.RequestURI == "/" {
			r.RequestURI = "/index.html"
		}

		http.FileServer(fs).ServeHTTP(w, r)
	}

	return fn
}

// sendPHPCORSHeaders sets CORS headers matching the PHP backend's ?cors parameter behavior.
// This is for API compatibility with the PHP version; the global CORS middleware already handles CORS.
func sendPHPCORSHeaders(w http.ResponseWriter, r *http.Request) {
	if r.FormValue("cors") == "true" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Encoding, Content-Type")
	}
}

func empty(w http.ResponseWriter, r *http.Request) {
	_, err := io.Copy(ioutil.Discard, r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	_ = r.Body.Close()

	sendPHPCORSHeaders(w, r)
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
}

func garbage(w http.ResponseWriter, r *http.Request) {
	sendPHPCORSHeaders(w, r)
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
			log.Warnf("Will use default value %d", chunks)
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
			// Client disconnects are expected during a speed test: the browser
			// aborts its download streams when the timed test ends. Don't spam
			// the log for those — only surface genuinely unexpected errors.
			if !isClientGone(err) {
				log.Errorf("Error writing back to client at chunk number %d: %s", i, err)
			}
			break
		}
	}
}

// isClientGone reports whether err is a normal client-side disconnect (the peer
// closed the connection / aborted the HTTP2 stream), which happens routinely
// when a speed test finishes and is not a server error.
func isClientGone(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "stream closed") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "client disconnected") ||
		strings.Contains(msg, "context canceled")
}

func getIP(w http.ResponseWriter, r *http.Request) {
	var ret results.Result

	clientIP := getClientIP(r)

	// Add anti-cache headers matching PHP behavior
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0, s-maxage=0")
	w.Header().Add("Cache-Control", "post-check=0, pre-check=0")
	w.Header().Set("Pragma", "no-cache")

	sendPHPCORSHeaders(w, r)

	if desc := classifyPrivateIP(clientIP); desc != "" {
		ret.ProcessedString = clientIP + " - " + desc
		b, _ := json.Marshal(&ret)
		if _, err := w.Write(b); err != nil {
			if !isClientGone(err) {
				log.Errorf("Error writing to client: %s", err)
			}
		}
		return
	}

	getISPInfo := r.FormValue("isp") == "true"
	distanceUnit := r.FormValue("distance")

	ret.ProcessedString = clientIP

	if getISPInfo {
		ispInfo := getISPInfoByPriority(clientIP)
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

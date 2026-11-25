package results

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/render"
	"github.com/librespeed/speedtest-go/config"
	"github.com/librespeed/speedtest-go/database"
	log "github.com/sirupsen/logrus"
)

// formatValue formats a numeric string for display, matching PHP behavior:
//   - values < 10: 2 decimal places
//   - values < 100: 1 decimal place
//   - values >= 100: 0 decimal places
func formatValue(d string) string {
	val, err := strconv.ParseFloat(d, 64)
	if err != nil {
		return d
	}
	if val < 10 {
		return strconv.FormatFloat(val, 'f', 2, 64)
	}
	if val < 100 {
		return strconv.FormatFloat(val, 'f', 1, 64)
	}
	return strconv.FormatFloat(val, 'f', 0, 64)
}

// extractISPName extracts the ISP name from the processedString format:
// "IP - ISP, Country (distance)" → "ISP"
func extractISPName(processedString string) string {
	dash := strings.Index(processedString, "-")
	if dash == -1 {
		return ""
	}
	isp := strings.TrimSpace(processedString[dash+1:])
	par := strings.LastIndex(isp, "(")
	if par != -1 {
		isp = strings.TrimSpace(isp[:par])
	}
	return isp
}

func exportJSONISPName(processedString string, disableIPInfo bool) string {
	if disableIPInfo {
		return ""
	}
	return extractISPName(processedString)
}

// JSONResponse is the structure returned by the JSON results endpoint
type JSONResponse struct {
	Timestamp string `json:"timestamp"`
	Download  string `json:"download"`
	Upload    string `json:"upload"`
	Ping      string `json:"ping"`
	Jitter    string `json:"jitter"`
	ISPInfo   string `json:"ispinfo"`
}

// JSONResult handles GET /results/json?id=X and returns test results as JSON
func JSONResult(w http.ResponseWriter, r *http.Request) {
	conf := config.LoadedConfig()

	if conf.DatabaseType == "none" {
		render.PlainText(w, r, "Telemetry is disabled")
		return
	}

	rawID := r.FormValue("id")
	if rawID == "" {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "missing id parameter"})
		return
	}

	uuid := ResolveID(rawID)
	record, err := database.DB.FetchByUUID(uuid)
	if err != nil {
		log.Errorf("Error querying database for JSON result: %s", err)
		w.WriteHeader(http.StatusNotFound)
		render.JSON(w, r, map[string]string{"error": "result not found"})
		return
	}

	// Format values for display (matching PHP json.php behavior)
	resp := JSONResponse{
		Timestamp: record.Timestamp.Format("2006-01-02 15:04:05"),
		Download:  formatValue(record.Download),
		Upload:    formatValue(record.Upload),
		Ping:      formatValue(record.Ping),
		Jitter:    formatValue(record.Jitter),
	}

	// Extract ISP name from ISP info JSON
	var result Result
	if !conf.DisableIPInfo {
		if err := json.Unmarshal([]byte(record.ISPInfo), &result); err == nil {
			resp.ISPInfo = exportJSONISPName(result.ProcessedString, false)
		}
	}

	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0, s-maxage=0")
	w.Header().Add("Cache-Control", "post-check=0, pre-check=0")
	w.Header().Set("Pragma", "no-cache")
	render.JSON(w, r, resp)
}

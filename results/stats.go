package results

import (
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/render"
	log "github.com/sirupsen/logrus"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/librespeed/speedtest-go/config"
	"github.com/librespeed/speedtest-go/database"
	"github.com/librespeed/speedtest-go/database/schema"
)

type StatsData struct {
	NoPassword    bool
	LoggedIn      bool
	Data          []schema.TelemetryData
	UniqueDevices int
	UniqueIPs     map[string]bool
	DeviceGroups  []DeviceGroup

	// pagination
	TotalTests  int
	CurrentPage int
	TotalPages  int
	PerPage     int
	Filters     SearchFilters
	FilterQuery string // active filter params as "&ip=..&dl=.." for pagination links
}

// DeviceGroup holds all tests that share the same browser-generated ClientID,
// so repeated tests from one machine (even across different IPs) stay together.
type DeviceGroup struct {
	ClientID  string
	Tests     []schema.TelemetryData
	TestCount int
	IPs       []string
}

// SearchFilters holds the advanced-search form fields. Text fields (IP, UUID)
// match as substrings; numeric/date fields compare against an operator.
type SearchFilters struct {
	UUID      string
	IP        string
	DL        string
	DLOp      string
	UL        string
	ULOp      string
	Ping      string
	PingOp    string
	Jitter    string
	JitterOp  string
	Date      string
	DateOp    string
}

// HasAny reports whether any filter field is set.
func (f SearchFilters) HasAny() bool {
	return f.UUID != "" || f.IP != "" || f.DL != "" || f.UL != "" ||
		f.Ping != "" || f.Jitter != "" || f.Date != ""
}

// QueryString builds a URL query suffix (starting with "&") carrying all
// active filters, so pagination links preserve the current search.
func (f SearchFilters) QueryString() string {
	var b strings.Builder
	appendParam := func(key, val, opKey, op string) {
		if val != "" {
			b.WriteString("&" + key + "=" + url.QueryEscape(val))
			if opKey != "" {
				b.WriteString("&" + opKey + "=" + op)
			}
		}
	}
	appendParam("uuid", f.UUID, "", "")
	appendParam("ip", f.IP, "", "")
	appendParam("dl", f.DL, "dl_op", f.DLOp)
	appendParam("ul", f.UL, "ul_op", f.ULOp)
	appendParam("ping", f.Ping, "ping_op", f.PingOp)
	appendParam("jitter", f.Jitter, "jitter_op", f.JitterOp)
	appendParam("date", f.Date, "date_op", f.DateOp)
	return b.String()
}

// compareOp evaluates a string operator ("ge","gt","eq","lt","le") against two
// float64 values parsed from strings. Returns true if the record value passes.
func compareOp(op string, recordStr, queryStr string) bool {
	rec, err1 := strconv.ParseFloat(strings.TrimSpace(recordStr), 64)
	q, err2 := strconv.ParseFloat(strings.TrimSpace(queryStr), 64)
	if err1 != nil || err2 != nil {
		return false
	}
	switch op {
	case "ge":
		return rec >= q
	case "gt":
		return rec > q
	case "lt":
		return rec < q
	case "le":
		return rec <= q
	default: // "eq"
		return rec == q
	}
}

// compareDate evaluates a date operator against the record's timestamp.
// Dates are compared as YYYY-MM-DD strings (lexicographic order == chronological).
func compareDate(op string, recordTS, queryDate string) bool {
	rec := recordTS[:10] // first 10 chars = YYYY-MM-DD
	switch op {
	case "ge":
		return rec >= queryDate
	case "gt":
		return rec > queryDate
	case "lt":
		return rec < queryDate
	case "le":
		return rec <= queryDate
	default: // "eq"
		return rec == queryDate
	}
}

// matches reports whether a single record passes all active filters.
func (f SearchFilters) matches(d schema.TelemetryData) bool {
	if f.UUID != "" && !strings.Contains(d.UUID, f.UUID) {
		return false
	}
	if f.IP != "" && !strings.Contains(d.IPAddress, f.IP) {
		return false
	}
	if f.DL != "" && !compareOp(f.DLOp, d.Download, f.DL) {
		return false
	}
	if f.UL != "" && !compareOp(f.ULOp, d.Upload, f.UL) {
		return false
	}
	if f.Ping != "" && !compareOp(f.PingOp, d.Ping, f.Ping) {
		return false
	}
	if f.Jitter != "" && !compareOp(f.JitterOp, d.Jitter, f.Jitter) {
		return false
	}
	if f.Date != "" {
		if !compareDate(f.DateOp, d.Timestamp.Format("2006-01-02 15:04:05"), f.Date) {
			return false
		}
	}
	return true
}

var (
	key   = []byte(securecookie.GenerateRandomKey(32))
	store = sessions.NewCookieStore(key)
)

func init() {
	store.Options = &sessions.Options{
		MaxAge:   3600 * 1, // 1 hour
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
}

func Stats(w http.ResponseWriter, r *http.Request) {
	// IMPORTANT: call LoadedConfig() inside the function, NOT as a package var.
	// A package-level var would snapshot the config at import time (BEFORE main()
	// loads settings.toml), freezing the defaults.
	conf := config.LoadedConfig()
	store.Options.Path = conf.BaseURL + "/stats"

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t, err := template.New("template").Funcs(template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
	}).Parse(statsTemplate)
	if err != nil {
		log.Errorf("Failed to parse template: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if conf.DatabaseType == "none" {
		render.PlainText(w, r, "Statistics are disabled")
		return
	}

	var data StatsData

	// Check if password is properly configured (not default value)
	if conf.StatsPassword == "" || conf.StatsPassword == "PASSWORD" {
		data.NoPassword = true
	}

	if !data.NoPassword {
		op := r.FormValue("op")
		session, _ := store.Get(r, "logged")
		auth, ok := session.Values["authenticated"].(bool)

		if auth && ok {
			if op == "logout" {
				session.Values["authenticated"] = false
				session.Options.MaxAge = -1
				session.Save(r, w)
				http.Redirect(w, r, conf.BaseURL+"/stats", http.StatusTemporaryRedirect)
			} else {
				data.LoggedIn = true

				const perPage = 50
				data.PerPage = perPage

				// page number (shared by browse and filtered views)
				page := 1
				if p := r.FormValue("page"); p != "" {
					if v, err := strconv.Atoi(p); err == nil && v > 0 {
						page = v
					}
				}

				// collect advanced-search filters
				filters := SearchFilters{
					UUID:     strings.TrimSpace(r.FormValue("uuid")),
					IP:       strings.TrimSpace(r.FormValue("ip")),
					DL:       strings.TrimSpace(r.FormValue("dl")),
					DLOp:     r.FormValue("dl_op"),
					UL:       strings.TrimSpace(r.FormValue("ul")),
					ULOp:     r.FormValue("ul_op"),
					Ping:     strings.TrimSpace(r.FormValue("ping")),
					PingOp:   r.FormValue("ping_op"),
					Jitter:   strings.TrimSpace(r.FormValue("jitter")),
					JitterOp: r.FormValue("jitter_op"),
					Date:     strings.TrimSpace(r.FormValue("date")),
					DateOp:   r.FormValue("date_op"),
				}
				data.Filters = filters

				// build a query suffix carrying the active filters so pagination
				// links keep the search context across pages.
				data.FilterQuery = filters.QueryString()

				if filters.HasAny() {
					// advanced search: load all records, filter in memory, paginate
					const maxScan = 100000
					all, err := database.DB.FetchAll(0, maxScan)
					if err != nil {
						log.Errorf("Error fetching data from database: %s", err)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					filtered := make([]schema.TelemetryData, 0, len(all))
					for _, d := range all {
						if filters.matches(d) {
							filtered = append(filtered, d)
						}
					}

					total := len(filtered)
					totalPages := (total + perPage - 1) / perPage
					if totalPages == 0 {
						totalPages = 1
					}
					if page > totalPages {
						page = totalPages
					}
					offset := (page - 1) * perPage
					end := offset + perPage
					if end > total {
						end = total
					}
					if offset > total {
						offset = total
					}
					data.Data = filtered[offset:end]
					data.TotalTests = total
					data.CurrentPage = page
					data.TotalPages = totalPages
				} else {
					// normal paginated browse of all tests (newest first)
					total, err := database.DB.Count()
					if err != nil {
						log.Errorf("Error counting database records: %s", err)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					totalPages := (total + perPage - 1) / perPage
					if totalPages == 0 {
						totalPages = 1
					}
					if page > totalPages {
						page = totalPages
					}

					offset := (page - 1) * perPage
					stats, err := database.DB.FetchAll(offset, perPage)
					if err != nil {
						log.Errorf("Error fetching data from database: %s", err)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}

					data.Data = stats
					data.TotalTests = total
					data.CurrentPage = page
					data.TotalPages = totalPages
				}

				// group tests by device (ClientID) and count unique IPs
				seen := map[string]bool{}
				data.UniqueIPs = map[string]bool{}
				groupOrder := []string{}
				groups := map[string]*DeviceGroup{}
				ungrouped := []schema.TelemetryData{}

				for _, d := range data.Data {
					if d.IPAddress != "" {
						data.UniqueIPs[d.IPAddress] = true
					}
					if d.ClientID == "" {
						ungrouped = append(ungrouped, d)
						continue
					}
					if _, ok := groups[d.ClientID]; !ok {
						groups[d.ClientID] = &DeviceGroup{ClientID: d.ClientID}
						groupOrder = append(groupOrder, d.ClientID)
						seen[d.ClientID] = true
					}
					g := groups[d.ClientID]
					g.Tests = append(g.Tests, d)
					g.TestCount++
					if d.IPAddress != "" {
						ipExists := false
						for _, ip := range g.IPs {
							if ip == d.IPAddress {
								ipExists = true
								break
							}
						}
						if !ipExists {
							g.IPs = append(g.IPs, d.IPAddress)
						}
					}
				}
				data.UniqueDevices = len(seen)

				// build ordered list of groups (most recent first), then ungrouped tests
				for _, cid := range groupOrder {
					data.DeviceGroups = append(data.DeviceGroups, *groups[cid])
				}
				if len(ungrouped) > 0 {
					data.DeviceGroups = append(data.DeviceGroups, DeviceGroup{
						ClientID:  "",
						Tests:     ungrouped,
						TestCount: len(ungrouped),
					})
				}
			}
		} else {
			if op == "login" {
				session, _ := store.Get(r, "logged")
				password := r.FormValue("password")
				if password == conf.StatsPassword {
					session.Values["authenticated"] = true
					session.Save(r, w)
					http.Redirect(w, r, conf.BaseURL+"/stats", http.StatusTemporaryRedirect)
				} else {
					w.WriteHeader(http.StatusForbidden)
				}
			}
		}
	}

	if err := t.Execute(w, data); err != nil {
		log.Errorf("Error executing template: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

const statsTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Speed Test Admin - Statistics</title>
	<style>
		* { margin: 0; padding: 0; box-sizing: border-box; }

		body {
			font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
			background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
			min-height: 100vh;
			padding: 2rem 1rem;
		}

		.container {
			max-width: 1400px;
			margin: 0 auto;
		}

		.header {
			background: rgba(255, 255, 255, 0.1);
			backdrop-filter: blur(10px);
			border-radius: 16px;
			padding: 2rem;
			margin-bottom: 2rem;
			display: flex;
			justify-content: space-between;
			align-items: center;
			flex-wrap: wrap;
			gap: 1rem;
		}

		.header h1 {
			color: white;
			font-weight: 600;
			font-size: 1.8rem;
		}

		.header .subtitle {
			color: rgba(255, 255, 255, 0.8);
			font-size: 0.9rem;
		}

		.login-form {
			background: white;
			border-radius: 16px;
			padding: 2rem;
			max-width: 400px;
			margin: 4rem auto;
			box-shadow: 0 10px 40px rgba(0, 0, 0, 0.1);
		}

		.login-form h2 {
			margin-bottom: 1.5rem;
			color: #333;
		}

		.login-form input {
			width: 100%;
			padding: 0.8rem;
			margin-bottom: 1rem;
			border: 1px solid #ddd;
			border-radius: 8px;
			font-size: 1rem;
		}

		.login-form button {
			width: 100%;
			padding: 0.8rem;
			background: #667eea;
			color: white;
			border: none;
			border-radius: 8px;
			font-size: 1rem;
			cursor: pointer;
			transition: background 0.3s;
		}

		.login-form button:hover {
			background: #5568d3;
		}

		.search-form {
			background: rgba(255, 255, 255, 0.1);
			backdrop-filter: blur(10px);
			border-radius: 16px;
			padding: 1.5rem;
			margin-bottom: 2rem;
			display: flex;
			flex-direction: column;
			gap: 0.75rem;
		}

		.filter-row {
			display: flex;
			gap: 1rem;
			flex-wrap: wrap;
			align-items: flex-end;
		}

		.filter-field {
			display: flex;
			flex-direction: column;
			gap: 0.3rem;
			flex: 1;
			min-width: 140px;
		}

		.filter-field label {
			color: rgba(255, 255, 255, 0.7);
			font-size: 0.72rem;
			font-weight: 600;
			text-transform: uppercase;
			letter-spacing: 0.05em;
		}

		.filter-field input,
		.filter-field select {
			padding: 0.6rem 0.7rem;
			border: none;
			border-radius: 8px;
			font-size: 0.95rem;
			background: white;
			color: #333;
		}

		.filter-compare {
			flex-direction: row;
			align-items: center;
			gap: 0.4rem;
			flex-wrap: nowrap;
		}

		.filter-compare label {
			white-space: nowrap;
		}

		.filter-compare select {
			width: 70px;
			flex: 0 0 auto;
			padding: 0.6rem 0.4rem;
		}

		.filter-compare input {
			flex: 1;
			min-width: 90px;
		}

		.filter-submit {
			padding: 0.6rem 1.8rem;
			background: #22d3ee;
			color: #0a0c10;
			border: none;
			border-radius: 8px;
			font-size: 0.95rem;
			font-weight: 600;
			cursor: pointer;
			transition: all 0.2s;
			align-self: flex-end;
			height: fit-content;
		}

		.filter-submit:hover {
			background: #1ebbd4;
			transform: translateY(-1px);
		}

		.badge-filtered {
			background: rgba(251, 191, 36, 0.3);
		}
		}

		.logout-btn {
			padding: 0.8rem 1.5rem;
			background: rgba(255, 255, 255, 0.2);
			color: white;
			border: none;
			border-radius: 8px;
			cursor: pointer;
			font-size: 1rem;
			transition: background 0.3s;
		}

		.logout-btn:hover {
			background: rgba(255, 255, 255, 0.3);
		}

		.results-grid {
			display: grid;
			grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
			gap: 1.5rem;
		}

		.device-group {
			margin-bottom: 2rem;
			background: rgba(255, 255, 255, 0.05);
			border: 1px solid rgba(255, 255, 255, 0.1);
			border-radius: 16px;
			padding: 1.25rem;
		}

		.device-header {
			display: flex;
			align-items: center;
			gap: 0.75rem;
			flex-wrap: wrap;
			padding-bottom: 1rem;
			margin-bottom: 1rem;
			border-bottom: 1px solid rgba(255, 255, 255, 0.1);
		}

		.device-title {
			font-weight: 600;
			color: #22d3ee;
			font-size: 1rem;
		}

		.device-header .device-id {
			font-family: monospace;
			font-size: 0.8rem;
			color: rgba(255, 255, 255, 0.6);
			background: rgba(255, 255, 255, 0.08);
			padding: 0.25rem 0.6rem;
			border-radius: 6px;
		}

		.device-meta {
			margin-left: auto;
			font-size: 0.8rem;
			color: rgba(255, 255, 255, 0.55);
		}

		.result-card {
			background: white;
			border-radius: 16px;
			padding: 1.5rem;
			box-shadow: 0 10px 40px rgba(0, 0, 0, 0.1);
			transition: transform 0.3s, box-shadow 0.3s;
		}

		.result-card:hover {
			transform: translateY(-5px);
			box-shadow: 0 15px 50px rgba(0, 0, 0, 0.15);
		}

		.result-card { cursor: pointer; }

		#result-modal {
			position: fixed;
			top: 0; left: 0;
			width: 100%; height: 100%;
			background: rgba(0, 0, 0, 0.8);
			backdrop-filter: blur(4px);
			z-index: 1000;
			display: flex;
			align-items: center;
			justify-content: center;
			padding: 2rem;
		}

		.modal-content {
			position: relative;
			width: 100%;
			max-width: 1200px;
			height: 90vh;
			background: #0a0c10;
			border-radius: 16px;
			overflow: hidden;
			box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
		}

		.modal-close {
			position: absolute;
			top: 0.75rem; right: 0.75rem;
			z-index: 10;
			width: 2.5rem; height: 2.5rem;
			border: none;
			border-radius: 50%;
			background: rgba(255, 255, 255, 0.15);
			color: white;
			font-size: 1.5rem;
			cursor: pointer;
			display: flex;
			align-items: center;
			justify-content: center;
			transition: background 0.2s;
		}

		.modal-close:hover {
			background: rgba(255, 255, 255, 0.3);
		}

		#result-frame {
			width: 100%;
			height: 100%;
			border: none;
		}

		.result-card .header {
			background: none;
			padding: 0;
			margin-bottom: 1rem;
			display: flex;
			justify-content: space-between;
			align-items: start;
		}

		.result-card .timestamp {
			color: #666;
			font-size: 0.8rem;
		}

		.result-card .test-id {
			color: #667eea;
			font-size: 0.75rem;
			font-weight: 600;
		}

		.metrics {
			display: grid;
			grid-template-columns: 1fr 1fr;
			gap: 0.5rem;
		}

		.metric {
			padding: 0.5rem;
			background: #f8f9fa;
			border-radius: 8px;
		}

		.metric .label {
			font-size: 0.7rem;
			color: #666;
			text-transform: uppercase;
			letter-spacing: 0.05em;
		}

		.metric .value {
			font-size: 1.2rem;
			font-weight: 600;
			color: #333;
		}

		.metric.download .value { color: #22d3ee; }
		.metric.upload .value { color: #a78bfa; }
		.metric.ping .value { color: #34d399; }
		.metric.jitter .value { color: #fbbf24; }

		.details {
			margin-top: 1rem;
			padding-top: 1rem;
			border-top: 1px solid #eee;
			font-size: 0.85rem;
			color: #666;
		}

		.details .device-id {
			font-family: monospace;
			font-size: 0.75rem;
			color: #888;
			margin-top: 0.3rem;
			word-break: break-all;
		}

		.stats-summary {
			margin-top: 0.8rem;
			display: flex;
			gap: 0.5rem;
			flex-wrap: wrap;
		}

		.badge {
			background: rgba(255, 255, 255, 0.15);
			color: white;
			padding: 0.3rem 0.8rem;
			border-radius: 999px;
			font-size: 0.8rem;
			font-weight: 500;
		}

		.badge-device {
			background: rgba(34, 211, 238, 0.25);
		}

		.badge-link {
			background: rgba(255, 255, 255, 0.1);
			text-decoration: none;
			color: white;
			cursor: pointer;
		}

		.pagination {
			display: flex;
			align-items: center;
			justify-content: center;
			gap: 1rem;
			margin-top: 2rem;
			padding: 1rem 0;
			flex-wrap: wrap;
		}

		.page-btn {
			padding: 0.6rem 1.4rem;
			background: rgba(255, 255, 255, 0.15);
			color: white;
			border-radius: 8px;
			text-decoration: none;
			font-weight: 600;
			cursor: pointer;
			transition: background 0.2s;
		}

		.page-btn:hover {
			background: rgba(34, 211, 238, 0.4);
		}

		.page-btn.disabled {
			opacity: 0.35;
			cursor: default;
			pointer-events: none;
		}

		.page-info {
			color: rgba(255, 255, 255, 0.8);
			font-size: 0.95rem;
		}

		.no-password {
			background: white;
			border-radius: 16px;
			padding: 3rem;
			text-align: center;
			box-shadow: 0 10px 40px rgba(0, 0, 0, 0.1);
		}

		.no-password h2 {
			color: #333;
			margin-bottom: 1rem;
		}

		.no-password p {
			color: #666;
		}

		@media (max-width: 768px) {
			.results-grid {
				grid-template-columns: 1fr;
			}

			.metrics {
				grid-template-columns: 1fr;
			}
		}
	</style>
</head>
<body>
	<div class="container">
		{{ if .NoPassword }}
			<div class="no-password">
				<h2>Statistics Disabled</h2>
				<p>Please set statistics_password in settings.toml to enable access.</p>
			</div>
		{{ else if .LoggedIn }}
			<div class="header">
				<div>
					<h1>🚀 Speed Test Admin</h1>
					<p class="subtitle">View and manage test results</p>
					<div class="stats-summary">
						{{ if .Filters.HasAny }}
							<span class="badge badge-filtered">🔎 filtered: {{ .TotalTests }} match{{ if gt .TotalTests 1 }}es{{ end }}</span>
							<a href="stats" class="badge badge-link">← clear filters</a>
						{{ else }}
							<span class="badge">📊 {{ .TotalTests }} total test{{ if gt .TotalTests 1 }}s{{ end }}</span>
							{{ if gt .UniqueDevices 0 }}<span class="badge badge-device">🖥 {{ .UniqueDevices }} unique device{{ if gt .UniqueDevices 1 }}s{{ end }}</span>{{ end }}
							<span class="badge">{{ len .UniqueIPs }} unique IP{{ if gt (len .UniqueIPs) 1 }}s{{ end }} on this page</span>
						{{ end }}
					</div>
				</div>
				<form action="stats" method="GET">
					<input type="hidden" name="op" value="logout" />
					<button type="submit" class="logout-btn">Logout</button>
				</form>
			</div>

			<form action="stats" method="GET" class="search-form">
				<div class="filter-row">
					<div class="filter-field">
						<label>Test ID</label>
						<input type="text" name="uuid" placeholder="UUID contains" value="{{ .Filters.UUID }}"/>
					</div>
					<div class="filter-field">
						<label>IP address</label>
						<input type="text" name="ip" placeholder="IP contains" value="{{ .Filters.IP }}"/>
					</div>
				</div>
				<div class="filter-row">
					<div class="filter-field filter-compare">
						<label>Date</label>
						<select name="date_op">
							<option value="eq"{{ if eq .Filters.DateOp "eq" }} selected{{ end }}>equals</option>
							<option value="ge"{{ if eq .Filters.DateOp "ge" }} selected{{ end }}>≥</option>
							<option value="le"{{ if eq .Filters.DateOp "le" }} selected{{ end }}>≤</option>
							<option value="gt"{{ if eq .Filters.DateOp "gt" }} selected{{ end }}>&gt;</option>
							<option value="lt"{{ if eq .Filters.DateOp "lt" }} selected{{ end }}>&lt;</option>
						</select>
						<input type="date" name="date" value="{{ .Filters.Date }}"/>
					</div>
					<div class="filter-field filter-compare">
						<label>Download</label>
						<select name="dl_op">
							<option value="ge"{{ if eq .Filters.DLOp "ge" }} selected{{ end }}>≥</option>
							<option value="eq"{{ if eq .Filters.DLOp "eq" }} selected{{ end }}>=</option>
							<option value="le"{{ if eq .Filters.DLOp "le" }} selected{{ end }}>≤</option>
							<option value="gt"{{ if eq .Filters.DLOp "gt" }} selected{{ end }}>&gt;</option>
							<option value="lt"{{ if eq .Filters.DLOp "lt" }} selected{{ end }}>&lt;</option>
						</select>
						<input type="number" step="0.01" name="dl" placeholder="Mbps" value="{{ .Filters.DL }}"/>
					</div>
					<div class="filter-field filter-compare">
						<label>Upload</label>
						<select name="ul_op">
							<option value="ge"{{ if eq .Filters.ULOp "ge" }} selected{{ end }}>≥</option>
							<option value="eq"{{ if eq .Filters.ULOp "eq" }} selected{{ end }}>=</option>
							<option value="le"{{ if eq .Filters.ULOp "le" }} selected{{ end }}>≤</option>
							<option value="gt"{{ if eq .Filters.ULOp "gt" }} selected{{ end }}>&gt;</option>
							<option value="lt"{{ if eq .Filters.ULOp "lt" }} selected{{ end }}>&lt;</option>
						</select>
						<input type="number" step="0.01" name="ul" placeholder="Mbps" value="{{ .Filters.UL }}"/>
					</div>
				</div>
				<div class="filter-row">
					<div class="filter-field filter-compare">
						<label>Ping</label>
						<select name="ping_op">
							<option value="le"{{ if eq .Filters.PingOp "le" }} selected{{ end }}>≤</option>
							<option value="eq"{{ if eq .Filters.PingOp "eq" }} selected{{ end }}>=</option>
							<option value="ge"{{ if eq .Filters.PingOp "ge" }} selected{{ end }}>≥</option>
							<option value="lt"{{ if eq .Filters.PingOp "lt" }} selected{{ end }}>&lt;</option>
							<option value="gt"{{ if eq .Filters.PingOp "gt" }} selected{{ end }}>&gt;</option>
						</select>
						<input type="number" step="0.01" name="ping" placeholder="ms" value="{{ .Filters.Ping }}"/>
					</div>
					<div class="filter-field filter-compare">
						<label>Jitter</label>
						<select name="jitter_op">
							<option value="le"{{ if eq .Filters.JitterOp "le" }} selected{{ end }}>≤</option>
							<option value="eq"{{ if eq .Filters.JitterOp "eq" }} selected{{ end }}>=</option>
							<option value="ge"{{ if eq .Filters.JitterOp "ge" }} selected{{ end }}>≥</option>
							<option value="lt"{{ if eq .Filters.JitterOp "lt" }} selected{{ end }}>&lt;</option>
							<option value="gt"{{ if eq .Filters.JitterOp "gt" }} selected{{ end }}>&gt;</option>
						</select>
						<input type="number" step="0.01" name="jitter" placeholder="ms" value="{{ .Filters.Jitter }}"/>
					</div>
					<button type="submit" class="filter-submit">🔍 Search</button>
				</div>
			</form>

			{{ range .DeviceGroups }}
				<div class="device-group">
					<div class="device-header">
						{{ if .ClientID }}
							<span class="device-title">🖥 Device</span>
							<span class="device-id">{{ .ClientID }}</span>
						{{ else }}
							<span class="device-title">⚠ Unknown device</span>
							<span class="device-id">no client identifier</span>
						{{ end }}
						<span class="device-meta">{{ .TestCount }} test{{ if gt .TestCount 1 }}s{{ end }} · {{ len .IPs }} IP{{ if gt (len .IPs) 1 }}s{{ end }}{{ range .IPs }} · {{ . }}{{ end }}</span>
					</div>

					<div class="results-grid">
						{{ range .Tests }}
						<div class="result-card" onclick="openResult('{{ .UUID }}')">
							<div class="header">
								<div>
									<div class="timestamp">{{ .Timestamp }}</div>
									<div class="test-id">{{ .UUID }}</div>
								</div>
							</div>

							<div class="metrics">
								<div class="metric download">
									<div class="label">Download</div>
									<div class="value">{{ .Download }}</div>
									<div class="label">Mbps</div>
								</div>
								<div class="metric upload">
									<div class="label">Upload</div>
									<div class="value">{{ .Upload }}</div>
									<div class="label">Mbps</div>
								</div>
								<div class="metric ping">
									<div class="label">Ping</div>
									<div class="value">{{ .Ping }}</div>
									<div class="label">ms</div>
								</div>
								<div class="metric jitter">
									<div class="label">Jitter</div>
									<div class="value">{{ .Jitter }}</div>
									<div class="label">ms</div>
								</div>
							</div>

							<div class="details">
								<p><strong>IP:</strong> {{ .IPAddress }}</p>
							</div>
						</div>
						{{ end }}
					</div>
				</div>
			{{ end }}

			<div class="pagination">
				{{ if gt .CurrentPage 1 }}
					<a href="stats?page={{ sub .CurrentPage 1 }}{{ .FilterQuery }}" class="page-btn">← Prev</a>
				{{ else }}
					<span class="page-btn disabled">← Prev</span>
				{{ end }}

				<span class="page-info">Page {{ .CurrentPage }} of {{ .TotalPages }} <em>({{ .TotalTests }} results)</em></span>

				{{ if lt .CurrentPage .TotalPages }}
					<a href="stats?page={{ add .CurrentPage 1 }}{{ .FilterQuery }}" class="page-btn">Next →</a>
				{{ else }}
					<span class="page-btn disabled">Next →</span>
				{{ end }}
			</div>
		{{ else }}
			<div class="login-form">
				<h2>🔐 Admin Login</h2>
				<form action="stats?op=login" method="POST">
					<input type="password" name="password" placeholder="Enter password" value="" required/>
					<button type="submit">Login</button>
				</form>
			</div>
		{{ end }}
	</div>

	{{ if .LoggedIn }}
	<!-- Result modal -->
	<div id="result-modal" onclick="if(event.target===this)closeResult()" style="display:none;">
		<div class="modal-content">
			<button class="modal-close" onclick="closeResult()">&times;</button>
			<iframe id="result-frame" src="" title="Test Result"></iframe>
		</div>
	</div>
	{{ end }}

	{{ if .LoggedIn }}
	<script>
		function openResult(uuid) {
			// Resolve ID using same path as the share link, then load the view page
			document.getElementById('result-frame').src = 'results/view?id=' + uuid;
			document.getElementById('result-modal').style.display = 'flex';
			document.body.style.overflow = 'hidden';
		}
		function closeResult() {
			document.getElementById('result-modal').style.display = 'none';
			document.getElementById('result-frame').src = '';
			document.body.style.overflow = '';
		}
		document.addEventListener('keydown', (e) => {
			if (e.key === 'Escape') closeResult();
		});
	</script>
	{{ end }}
</body>
</html>`

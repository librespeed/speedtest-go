package results

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/go-chi/render"
	log "github.com/sirupsen/logrus"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/librespeed/speedtest-go/config"
	"github.com/librespeed/speedtest-go/database"
	"github.com/librespeed/speedtest-go/database/schema"
)

type StatsData struct {
	NoPassword   bool
	LoggedIn     bool
	Data         []schema.TelemetryData
	ConfigDebug  map[string]interface{}
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
	t, err := template.New("template").Parse(statsTemplate)
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

				id := r.FormValue("id")
				switch id {
				case "L100":
					stats, err := database.DB.FetchLast100()
					if err != nil {
						log.Errorf("Error fetching data from database: %s", err)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					data.Data = stats
				case "":
				default:
					stat, err := database.DB.FetchByUUID(id)
					if err != nil {
						log.Errorf("Error fetching data from database: %s", err)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					data.Data = append(data.Data, *stat)
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
			gap: 1rem;
			flex-wrap: wrap;
			align-items: center;
		}

		.search-form input {
			flex: 1;
			min-width: 200px;
			padding: 0.8rem;
			border: none;
			border-radius: 8px;
			font-size: 1rem;
		}

		.search-form button {
			padding: 0.8rem 1.5rem;
			background: #22d3ee;
			color: #0a0c10;
			border: none;
			border-radius: 8px;
			font-size: 1rem;
			font-weight: 600;
			cursor: pointer;
			transition: all 0.3s;
		}

		.search-form button:hover {
			background: #1ebbd4;
			transform: translateY(-2px);
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

				<div style="margin-top: 2rem; padding: 1rem; background: rgba(255,0,0,0.1); border: 1px solid rgba(255,0,0,0.3); border-radius: 8px;">
					<h3 style="color: #ff6b6b; margin-bottom: 1rem;">DEBUG INFO</h3>
					{{ range $key, $value := .ConfigDebug }}
						<p style="margin: 0.5rem 0; color: #e0e0e0;"><strong>{{ $key }}:</strong> {{ $value }}</p>
					{{ end }}
				</div>
			</div>
		{{ else if .LoggedIn }}
			<div class="header">
				<div>
					<h1>🚀 Speed Test Admin</h1>
					<p class="subtitle">View and manage test results</p>
				</div>
				<form action="stats" method="GET">
					<input type="hidden" name="op" value="logout" />
					<button type="submit" class="logout-btn">Logout</button>
				</form>
			</div>

			<form action="stats" method="GET" class="search-form">
				<input type="hidden" name="op" value="id" />
				<input type="text" name="id" id="id" placeholder="Enter Test ID or leave empty for last 100" value=""/>
				<button type="submit">Search</button>
				<button type="button" onclick="document.getElementById('id').value='L100'; document.forms[1].submit();">Show Last 100</button>
			</form>

			<div class="results-grid">
				{{ range $i, $v := .Data }}
				<div class="result-card" onclick="openResult('{{ $v.UUID }}')">
					<div class="header">
						<div>
							<div class="timestamp">{{ $v.Timestamp }}</div>
							<div class="test-id">{{ $v.UUID }}</div>
						</div>
					</div>

					<div class="metrics">
						<div class="metric download">
							<div class="label">Download</div>
							<div class="value">{{ $v.Download }}</div>
							<div class="label">Mbps</div>
						</div>
						<div class="metric upload">
							<div class="label">Upload</div>
							<div class="value">{{ $v.Upload }}</div>
							<div class="label">Mbps</div>
						</div>
						<div class="metric ping">
							<div class="label">Ping</div>
							<div class="value">{{ $v.Ping }}</div>
							<div class="label">ms</div>
						</div>
						<div class="metric jitter">
							<div class="label">Jitter</div>
							<div class="value">{{ $v.Jitter }}</div>
							<div class="label">ms</div>
						</div>
					</div>

					<div class="details">
						<p><strong>IP:</strong> {{ $v.IPAddress }}</p>
					</div>
				</div>
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

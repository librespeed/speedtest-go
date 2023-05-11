package results

import (
	"html/template"
	"net/http"

	"github.com/go-chi/render"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/librespeed/speedtest/config"
	"github.com/librespeed/speedtest/database"
	"github.com/librespeed/speedtest/database/schema"
)

type StatsData struct {
	NoPassword bool
	LoggedIn   bool
	Data       []schema.TelemetryData
}

var (
	store         *sessions.CookieStore
	conf          *config.Config
	checkPassword func(password string) bool
)

func statsInitialize(c *config.Config) {
	key := []byte(securecookie.GenerateRandomKey(32))
	store = sessions.NewCookieStore(key)
	store.Options = &sessions.Options{
		Path:     c.BaseURL + "/stats",
		MaxAge:   3600 * 1, // 1 hour
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	conf = c

	// Check if StatsPassword is a valid bcrypt hash
	if _, err := bcrypt.Cost([]byte(c.StatsPassword)); err == nil {
		log.Println("statistics_password is valid bcrypt hash")
		checkPassword = func(password string) bool {
			return nil == bcrypt.CompareHashAndPassword([]byte(c.StatsPassword), []byte(password))
		}

	} else {
		checkPassword = func(password string) bool {
			return password == c.StatsPassword
		}
	}
}

func Stats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t, err := template.New("template").Parse(htmlTemplate)
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

	if conf.StatsPassword == "PASSWORD" {
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
				if checkPassword(password) {
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

const htmlTemplate = `<!DOCTYPE html>
<html>
<head>
<title>LibreSpeed - Stats</title>
<style type="text/css">
	html,body{
		margin:0;
		padding:0;
		border:none;
		width:100%; min-height:100%;
	}
	html{
		background-color: hsl(198,72%,35%);
		font-family: "Segoe UI","Roboto",sans-serif;
	}
	body{
		background-color:#FFFFFF;
		box-sizing:border-box;
		width:100%;
		max-width:70em;
		margin:4em auto;
		box-shadow:0 1em 6em #00000080;
		padding:1em 1em 4em 1em;
		border-radius:0.4em;
	}
	h1,h2,h3,h4,h5,h6{
		font-weight:300;
		margin-bottom: 0.1em;
	}
	h1{
		text-align:center;
	}
	table{
		margin:2em 0;
		width:100%;
	}
	table, tr, th, td {
		border: 1px solid #AAAAAA;
	}
	th {
		width: 6em;
	}
	td {
		word-break: break-all;
	}
</style>
</head>
<body>
<h1>LibreSpeed - Stats</h1>
{{ if .NoPassword }}
		Please set statistics_password in settings.toml to enable access.
{{ else if .LoggedIn }}
	<form action="stats" method="GET"><input type="hidden" name="op" value="logout" /><input type="submit" value="Logout" /></form>
	<form action="stats" method="GET">
		<h3>Search test results</h6>
		<input type="hidden" name="op" value="id" />
		<input type="text" name="id" id="id" placeholder="Test ID" value=""/>
		<input type="submit" value="Find" />
		<input type="submit" onclick="document.getElementById('id').value='L100'" value="Show last 100 tests" />
	</form>

	{{ range $i, $v := .Data }}
	<table>
		<tr><th>Test ID</th><td>{{ $v.UUID }}</td></tr>
		<tr><th>Date and time</th><td>{{ $v.Timestamp }}</td></tr>
		<tr><th>IP and ISP Info</th><td>{{ $v.IPAddress }}<br/>{{ $v.ISPInfo }}</td></tr>
		<tr><th>User agent and locale</th><td>{{ $v.UserAgent }}<br/>{{ $v.Language }}</td></tr>
		<tr><th>Download speed</th><td>{{ $v.Download }}</td></tr>
		<tr><th>Upload speed</th><td>{{ $v.Upload }}</td></tr>
		<tr><th>Ping</th><td>{{ $v.Ping }}</td></tr>
		<tr><th>Jitter</th><td>{{ $v.Jitter }}</td></tr>
		<tr><th>Log</th><td>{{ $v.Log }}</td></tr>
		<tr><th>Extra info</th><td>{{ $v.Extra }}</td></tr>
	</table>
	{{ end }}
{{ else }}
	<form action="stats?op=login" method="POST">
		<h3>Login</h3>
		<input type="password" name="password" placeholder="Password" value=""/>
		<input type="submit" value="Login" />
	</form>
{{ end }}
</body>
</html>`

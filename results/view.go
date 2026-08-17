package results

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strings"

	"github.com/librespeed/speedtest-go/config"
	"github.com/librespeed/speedtest-go/database"
	log "github.com/sirupsen/logrus"
)

// gradeInfo holds the parsed grade fields for template rendering.
type gradeInfo struct {
	Grade string
}

// ViewPage renders a beautiful HTML page with test results
func ViewPage(w http.ResponseWriter, r *http.Request) {
	conf := config.LoadedConfig()
	if conf.DatabaseType == "none" {
		http.Error(w, "Database is disabled", http.StatusServiceUnavailable)
		return
	}

	rawID := r.FormValue("id")
	if rawID == "" {
		http.Error(w, "Missing test ID", http.StatusBadRequest)
		return
	}

	uuid := ResolveID(rawID)
	record, err := database.DB.FetchByUUID(uuid)
	if err != nil {
		http.Error(w, "Test not found", http.StatusNotFound)
		return
	}

	// Parse ISP info
	var ispInfo struct {
		ProcessedString string `json:"processedString"`
		RawISPInfo      struct {
			IP        string `json:"ip"`
			Hostname  string `json:"hostname"`
			City      string `json:"city"`
			Region    string `json:"region"`
			Country   string `json:"country"`
			Loc       string `json:"loc"`
			Org       string `json:"org"`
			Postal    string `json:"postal"`
			Timezone  string `json:"timezone"`
		} `json:"rawIspInfo"`
	}

	if record.ISPInfo != "" {
		if err := json.Unmarshal([]byte(record.ISPInfo), &ispInfo); err != nil {
			log.Errorf("Error parsing ISP info: %s", err)
		}
	}

	// Parse the HTML template
	t, err := template.New("results").Funcs(template.FuncMap{
		"lower": strings.ToLower,
	}).Parse(viewTemplate)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	// Parse grade data
	var grade gradeInfo
	if record.GradeData != "" {
		var gd struct {
			Grade string `json:"grade"`
		}
		if err := json.Unmarshal([]byte(record.GradeData), &gd); err == nil {
			grade.Grade = gd.Grade
		}
	}
	if grade.Grade == "" {
		grade.Grade = "—"
	}

	// ChartData is injected as raw JS — use template.JS to skip HTML escaping.
	chartJS := template.JS("{}")
	if record.ChartData != "" {
		chartJS = template.JS(record.ChartData)
	}

	// Prepare data for template
	data := struct {
		BaseURL   string
		Download  string
		Upload    string
		Ping      string
		Jitter    string
		Timestamp string
		IPAddress string
		ISP       struct {
			ProcessedString string
			City            string
			Region          string
			Country         string
			Organization    string
		}
		Grade   gradeInfo
		Latency string
		ChartJS template.JS
	}{
		BaseURL:   conf.BaseURL,
		Download:  record.Download,
		Upload:    record.Upload,
		Ping:      record.Ping,
		Jitter:    record.Jitter,
		Timestamp: record.Timestamp.Format("2006-01-02 15:04:05"),
		IPAddress: record.IPAddress,
		ISP: struct {
			ProcessedString string
			City            string
			Region          string
			Country         string
			Organization    string
		}{
			ProcessedString: ispInfo.ProcessedString,
			City:            ispInfo.RawISPInfo.City,
			Region:          ispInfo.RawISPInfo.Region,
			Country:         ispInfo.RawISPInfo.Country,
			Organization:    ispInfo.RawISPInfo.Org,
		},
		Grade:   grade,
		Latency: record.LatencyUnderload,
		ChartJS: chartJS,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, data); err != nil {
		http.Error(w, "Render error", http.StatusInternalServerError)
	}
}

const viewTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Speed Test Results</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }

        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            background: #0a0c10;
            color: #eef2ff;
            min-height: 100vh;
            padding: 2rem 1rem;
        }

        .container {
            max-width: 1200px;
            margin: 0 auto;
        }

        .header {
            text-align: center;
            margin-bottom: 2rem;
        }

        .header h1 {
            font-size: 2rem;
            font-weight: 300;
            color: #34d399;
        }

        .header .timestamp {
            color: #697282;
            font-size: 0.9rem;
            margin-top: 0.5rem;
        }

        .metrics {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 1.5rem;
            margin-bottom: 2rem;
        }

        .metric-card {
            background: #0e1016;
            border: 1px solid #181a20;
            border-radius: 14px;
            padding: 2rem 1.5rem;
            text-align: center;
        }

        .metric-value {
            font-size: 2.5rem;
            font-weight: 300;
            line-height: 1;
            margin-bottom: 0.5rem;
        }

        .metric-label {
            color: #697282;
            font-size: 0.75rem;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            margin-bottom: 0.25rem;
        }

        .metric-unit {
            color: #343848;
            font-size: 0.85rem;
        }

        .download .metric-value { color: #22d3ee; }
        .upload .metric-value { color: #a78bfa; }
        .ping .metric-value { color: #34d399; }
        .jitter .metric-value { color: #fbbf24; }

        .chart-section {
            background: #0e1016;
            border: 1px solid #181a20;
            border-radius: 14px;
            padding: 1.5rem;
            margin-bottom: 2rem;
        }

        .chart-section h2 {
            font-size: 1.2rem;
            font-weight: 400;
            margin-bottom: 1rem;
            color: #eef2ff;
        }

        #chartCanvas {
            width: 100%;
            height: 300px;
            background: #07090f;
            border-radius: 8px;
        }

        .grade-section {
            background: #0e1016;
            border: 1px solid #181a20;
            border-radius: 14px;
            padding: 1.5rem;
            margin-bottom: 2rem;
        }

        .grade-section h2 {
            font-size: 1.2rem;
            font-weight: 400;
            margin-bottom: 1rem;
            color: #eef2ff;
        }

        .grade-display {
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 2rem;
        }

        .grade-letter {
            font-size: 4rem;
            font-weight: 600;
        }

        .grade-a { color: #34d399; }
        .grade-b { color: #22d3ee; }
        .grade-c { color: #fbbf24; }
        .grade-d { color: #fb923c; }
        .grade-e { color: #f87171; }
        .grade-f { color: #ef4444; }

        .grade-details {
            color: #697282;
        }

        .info-section {
            background: #0e1016;
            border: 1px solid #181a20;
            border-radius: 14px;
            padding: 1.5rem;
            font-size: 0.9rem;
            color: #697282;
        }

        .info-section p {
            margin-bottom: 0.5rem;
        }

        .info-section h3 {
            color: #eef2ff;
            font-size: 1.1rem;
            margin-bottom: 1rem;
            font-weight: 500;
        }

        .info-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 1rem;
        }

        .info-item {
            display: flex;
            flex-direction: column;
            gap: 0.25rem;
        }

        .info-label {
            color: #697282;
            font-size: 0.8rem;
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }

        .info-value {
            color: #eef2ff;
            font-size: 0.95rem;
            font-weight: 500;
        }

        @media (max-width: 768px) {
            .metrics {
                grid-template-columns: repeat(2, 1fr);
            }

            .metric-value {
                font-size: 2rem;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Speed Test Results</h1>
            <div class="timestamp">{{ .Timestamp }}</div>
        </div>

        <div class="metrics">
            <div class="metric-card download">
                <div class="metric-label">Download</div>
                <div class="metric-value">{{ .Download }}</div>
                <div class="metric-unit">Mbps</div>
            </div>

            <div class="metric-card upload">
                <div class="metric-label">Upload</div>
                <div class="metric-value">{{ .Upload }}</div>
                <div class="metric-unit">Mbps</div>
            </div>

            <div class="metric-card ping">
                <div class="metric-label">Ping</div>
                <div class="metric-value">{{ .Ping }}</div>
                <div class="metric-unit">ms</div>
            </div>

            <div class="metric-card jitter">
                <div class="metric-label">Jitter</div>
                <div class="metric-value">{{ .Jitter }}</div>
                <div class="metric-unit">ms</div>
            </div>
        </div>

        <div class="chart-section">
            <h2>Speed Over Time</h2>
            <canvas id="chartCanvas"></canvas>
        </div>

        <div class="grade-section">
            <h2>Performance Grade</h2>
            <div class="grade-display">
                <div class="grade-letter grade-{{ .Grade.Grade | lower }}">{{ .Grade.Grade }}</div>
                {{ if .Latency }}
                <div class="grade-details">
                    <p>Latency under load: {{ .Latency }} ms</p>
                </div>
                {{ end }}
            </div>
        </div>

        <div class="info-section">
            <h3>Connection Information</h3>
            <div class="info-grid">
                <div class="info-item">
                    <span class="info-label">IP Address</span>
                    <span class="info-value">{{ .IPAddress }}</span>
                </div>
                {{ if .ISP.ProcessedString }}
                <div class="info-item">
                    <span class="info-label">Location</span>
                    <span class="info-value">{{ .ISP.ProcessedString }}</span>
                </div>
                {{ end }}
                {{ if .ISP.Organization }}
                <div class="info-item">
                    <span class="info-label">Organization</span>
                    <span class="info-value">{{ .ISP.Organization }}</span>
                </div>
                {{ end }}
                {{ if .ISP.City }}
                <div class="info-item">
                    <span class="info-label">City</span>
                    <span class="info-value">{{ .ISP.City }}</span>
                </div>
                {{ end }}
                {{ if .ISP.Region }}
                <div class="info-item">
                    <span class="info-label">Region</span>
                    <span class="info-value">{{ .ISP.Region }}</span>
                </div>
                {{ end }}
                {{ if .ISP.Country }}
                <div class="info-item">
                    <span class="info-label">Country</span>
                    <span class="info-value">{{ .ISP.Country }}</span>
                </div>
                {{ end }}
            </div>
        </div>
    </div>

    <script>
        // Chart drawing logic
        const canvas = document.getElementById('chartCanvas');
        const ctx = canvas.getContext('2d');

        function resizeCanvas() {
            canvas.width = canvas.offsetWidth;
            canvas.height = canvas.offsetHeight;
            drawChart();
        }

        const chartData = {{ .ChartJS }};
        const dlData = (chartData && chartData.dl) ? chartData.dl : [];
        const ulData = (chartData && chartData.ul) ? chartData.ul : [];

        function drawChart() {
            const width = canvas.width;
            const height = canvas.height;
            const padding = 40;

            ctx.fillStyle = '#07090f';
            ctx.fillRect(0, 0, width, height);

            // Compute scale
            const allPoints = dlData.concat(ulData);
            const maxT = allPoints.reduce((m, p) => Math.max(m, p.t), 1);
            const maxV = allPoints.reduce((m, p) => Math.max(m, p.v), 1);

            // Grid
            ctx.strokeStyle = '#181a20';
            ctx.lineWidth = 1;
            for (let i = 0; i <= 5; i++) {
                const y = padding + (height - 2 * padding) * i / 5;
                ctx.beginPath();
                ctx.moveTo(padding, y);
                ctx.lineTo(width - padding, y);
                ctx.stroke();
            }

            function drawLine(data, color) {
                if (!data.length) return;
                ctx.strokeStyle = color;
                ctx.lineWidth = 2;
                ctx.beginPath();
                data.forEach((point, i) => {
                    const x = padding + (point.t / maxT) * (width - 2 * padding);
                    const y = height - padding - (point.v / maxV) * (height - 2 * padding);
                    if (i === 0) ctx.moveTo(x, y);
                    else ctx.lineTo(x, y);
                });
                ctx.stroke();
            }

            drawLine(dlData, '#22d3ee');
            drawLine(ulData, '#a78bfa');
        }

        window.addEventListener('resize', resizeCanvas);
        resizeCanvas();
    </script>
</body>
</html>
`

package schema

import (
	"time"
)

type TelemetryData struct {
	Timestamp       time.Time
	IPAddress       string
	ISPInfo         string
	Extra           string
	UserAgent       string
	Language        string
	Download        string
	Upload          string
	Ping            string
	Jitter          string
	Log             string
	UUID            string
	GradeData       string   // JSON: {grade: "A", criteria: {...}}
	ChartData       string   // JSON: {dl: [{t, v}], ul: [{t, v}]}
	LatencyUnderload string   // ping during load (ms)
	PingDuringTest  string   // JSON: {dl: [...], ul: [...]}
	ClientID        string   // browser-generated stable device identifier
}

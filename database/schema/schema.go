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
	// ClientID is a self-reported, client-generated identifier (a random ID
	// persisted in localStorage, optionally anchored by a browser fingerprint
	// hash — see getClientIdentity() in index.html). It is NOT authenticated:
	// any client can send an arbitrary value, so it must never be used for
	// access control, rate limiting, or any other security decision. It only
	// exists to group repeat tests from the same browser on the (password-
	// gated) stats page, as a display convenience — treat it as a hint, not
	// a verified device identity.
	ClientID string
}

package schema

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// SearchFilters holds the advanced-search form fields used by the stats
// page. Text fields (IP, UUID) match as substrings; numeric/date fields
// compare against an operator ("ge","gt","eq","lt","le").
type SearchFilters struct {
	UUID     string
	IP       string
	DL       string
	DLOp     string
	UL       string
	ULOp     string
	Ping     string
	PingOp   string
	Jitter   string
	JitterOp string
	Date     string
	DateOp   string
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

// Matches reports whether a single record passes all active filters. Used by
// the backends (bolt, memory) that have no query engine of their own to push
// filtering into; SQL-backed drivers use BuildWhere instead.
func (f SearchFilters) Matches(d TelemetryData) bool {
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

var isoDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// sqlOp maps a compare-operator name to its SQL symbol, defaulting to "=".
func sqlOp(op string) string {
	switch op {
	case "ge":
		return ">="
	case "gt":
		return ">"
	case "lt":
		return "<"
	case "le":
		return "<="
	default:
		return "="
	}
}

// SQLDialect supplies the syntax differences BuildWhere needs to stay
// portable across backends: bind-parameter placeholders, and how to compare
// the text-typed numeric/timestamp columns as numbers/dates.
type SQLDialect struct {
	// Placeholder renders the nth (1-based) bind parameter in this dialect's
	// syntax (e.g. "?", "$1", "@p1").
	Placeholder func(n int) string
	// NumericCast wraps a column reference so it compares numerically
	// instead of lexicographically (dl/ul/ping/jitter are stored as TEXT).
	NumericCast func(col string) string
	// DateCast wraps the timestamp column reference down to its date part.
	DateCast func(col string) string
}

// BuildWhere returns a SQL WHERE-clause body (without the leading "WHERE",
// empty if there are no active filters) and its positional args, in the
// same order as the placeholders appear in the returned clause.
//
// A filter value that can't be parsed the way it needs to be (a non-numeric
// value for a numeric comparison, a malformed date) is treated as "matches
// nothing" — the same behavior the in-memory filter had — rather than being
// silently dropped (which would instead match everything).
func (f SearchFilters) BuildWhere(d SQLDialect) (string, []interface{}) {
	var clauses []string
	var args []interface{}

	bind := func(val interface{}) string {
		args = append(args, val)
		return d.Placeholder(len(args))
	}

	// Explicit ESCAPE clause: SQLite has no default LIKE escape character at
	// all, and MSSQL's default differs from Postgres/MySQL's backslash, so
	// this must be spelled out to behave the same everywhere.
	if f.UUID != "" {
		clauses = append(clauses, "uuid LIKE "+bind("%"+likeEscape(f.UUID)+"%")+" ESCAPE '\\'")
	}
	if f.IP != "" {
		clauses = append(clauses, "ip LIKE "+bind("%"+likeEscape(f.IP)+"%")+" ESCAPE '\\'")
	}
	addNumeric := func(col, op, val string) {
		q, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err != nil {
			clauses = append(clauses, "1=0")
			return
		}
		clauses = append(clauses, fmt.Sprintf("%s %s %s", d.NumericCast(col), sqlOp(op), bind(q)))
	}
	if f.DL != "" {
		addNumeric("dl", f.DLOp, f.DL)
	}
	if f.UL != "" {
		addNumeric("ul", f.ULOp, f.UL)
	}
	if f.Ping != "" {
		addNumeric("ping", f.PingOp, f.Ping)
	}
	if f.Jitter != "" {
		addNumeric("jitter", f.JitterOp, f.Jitter)
	}
	if f.Date != "" {
		if !isoDate.MatchString(f.Date) {
			clauses = append(clauses, "1=0")
		} else {
			clauses = append(clauses, fmt.Sprintf("%s %s %s", d.DateCast("timestamp"), sqlOp(f.DateOp), bind(f.Date)))
		}
	}

	return strings.Join(clauses, " AND "), args
}

// likeEscape escapes LIKE wildcard characters so a filter value like "50%"
// is matched literally instead of as a pattern.
func likeEscape(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_")
	return r.Replace(s)
}

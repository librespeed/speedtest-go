package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/librespeed/speedtest-go/database/schema"

	_ "modernc.org/sqlite"
	log "github.com/sirupsen/logrus"
)

type SQLite struct {
	db *sql.DB
}

func Open(databaseFile string) *SQLite {
	conn, err := sql.Open("sqlite", databaseFile)
	if err != nil {
		log.Fatalf("Cannot open SQLite database: %s", err)
	}

	// Enable WAL mode for better concurrent performance
	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		log.Warnf("Failed to set SQLite journal mode to WAL: %s", err)
	}

	// Create table if not exists
	stmt := `CREATE TABLE IF NOT EXISTS speedtest_users (
		id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		ip TEXT NOT NULL,
		ispinfo TEXT,
		extra TEXT,
		ua TEXT NOT NULL,
		lang TEXT NOT NULL,
		dl TEXT,
		ul TEXT,
		ping TEXT,
		jitter TEXT,
		log TEXT,
		uuid TEXT,
		grade_data TEXT,
		chart_data TEXT,
		latency_underload TEXT,
		ping_during_test TEXT,
		client_id TEXT
	);`
	if _, err := conn.Exec(stmt); err != nil {
		log.Fatalf("Failed to create speedtest_users table: %s", err)
	}

	// Migrate existing databases that predate the new columns.
	existing := map[string]bool{}
	rows, err := conn.Query(`PRAGMA table_info(speedtest_users)`)
	if err != nil {
		log.Fatalf("Failed to inspect speedtest_users schema: %s", err)
	}
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			rows.Close()
			log.Fatalf("Failed to read speedtest_users schema: %s", err)
		}
		existing[name] = true
	}
	rows.Close()

	for _, col := range []string{"grade_data", "chart_data", "latency_underload", "ping_during_test", "client_id"} {
		if existing[col] {
			continue
		}
		if _, err := conn.Exec(`ALTER TABLE speedtest_users ADD COLUMN ` + col + ` TEXT`); err != nil {
			log.Fatalf("Failed to add column %s to speedtest_users: %s", col, err)
		}
	}

	return &SQLite{db: conn}
}

func (p *SQLite) Insert(data *schema.TelemetryData) error {
	var existingID int
	// Check for duplicate UUID first
	err := p.db.QueryRow(`SELECT id FROM speedtest_users WHERE uuid = ?`, data.UUID).Scan(&existingID)
	if err == nil {
		// Record with this UUID already exists - skip insert
		return nil
	}

	stmt := `INSERT INTO speedtest_users
		(ip, ispinfo, extra, ua, lang, dl, ul, ping, jitter, log, uuid,
		 grade_data, chart_data, latency_underload, ping_during_test, client_id)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?);`
	_, err = p.db.Exec(stmt,
		data.IPAddress, data.ISPInfo, data.Extra, data.UserAgent, data.Language,
		data.Download, data.Upload, data.Ping, data.Jitter, data.Log, data.UUID,
		data.GradeData, data.ChartData, data.LatencyUnderload, data.PingDuringTest, data.ClientID)
	return err
}

func sqScan(row interface{ Scan(...any) error }, record *schema.TelemetryData) error {
	var id int
	return row.Scan(&id, &record.Timestamp, &record.IPAddress, &record.ISPInfo, &record.Extra,
		&record.UserAgent, &record.Language, &record.Download, &record.Upload,
		&record.Ping, &record.Jitter, &record.Log, &record.UUID,
		&record.GradeData, &record.ChartData, &record.LatencyUnderload,
		&record.PingDuringTest, &record.ClientID)
}

func (p *SQLite) FetchByUUID(uuid string) (*schema.TelemetryData, error) {
	var record schema.TelemetryData
	row := p.db.QueryRow(
		`SELECT id, timestamp, ip, ispinfo, extra, ua, lang, dl, ul, ping, jitter, log, uuid,
			COALESCE(grade_data,''), COALESCE(chart_data,''), COALESCE(latency_underload,''),
			COALESCE(ping_during_test,''), COALESCE(client_id,'')
		FROM speedtest_users WHERE uuid = ?`, uuid)
	if err := sqScan(row, &record); err != nil {
		return nil, fmt.Errorf("sqlite fetch by uuid: %w", err)
	}
	return &record, nil
}

func (p *SQLite) FetchLast100() ([]schema.TelemetryData, error) {
	rows, err := p.db.Query(
		`SELECT id, timestamp, ip, ispinfo, extra, ua, lang, dl, ul, ping, jitter, log, uuid,
			COALESCE(grade_data,''), COALESCE(chart_data,''), COALESCE(latency_underload,''),
			COALESCE(ping_during_test,''), COALESCE(client_id,'')
		FROM speedtest_users ORDER BY timestamp DESC, id DESC LIMIT 100`)
	if err != nil {
		return nil, fmt.Errorf("sqlite fetch last 100: %w", err)
	}
	defer rows.Close()
	var records []schema.TelemetryData
	for rows.Next() {
		var record schema.TelemetryData
		if err := sqScan(rows, &record); err != nil {
			return nil, fmt.Errorf("sqlite scan row: %w", err)
		}
		records = append(records, record)
	}
	return records, nil
}

func (p *SQLite) FetchAll(offset, limit int) ([]schema.TelemetryData, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := p.db.Query(
		`SELECT id, timestamp, ip, ispinfo, extra, ua, lang, dl, ul, ping, jitter, log, uuid,
			COALESCE(grade_data,''), COALESCE(chart_data,''), COALESCE(latency_underload,''),
			COALESCE(ping_during_test,''), COALESCE(client_id,'')
		FROM speedtest_users ORDER BY timestamp DESC, id DESC LIMIT ? OFFSET ?`,
		limit, offset)
	if err != nil {
		return nil, fmt.Errorf("sqlite fetch all: %w", err)
	}
	defer rows.Close()
	var records []schema.TelemetryData
	for rows.Next() {
		var record schema.TelemetryData
		if err := sqScan(rows, &record); err != nil {
			return nil, fmt.Errorf("sqlite scan row: %w", err)
		}
		records = append(records, record)
	}
	return records, nil
}

func (p *SQLite) Count() (int, error) {
	var count int
	err := p.db.QueryRow(`SELECT COUNT(*) FROM speedtest_users;`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("sqlite count: %w", err)
	}
	return count, nil
}

var sqliteDialect = schema.SQLDialect{
	Placeholder: func(int) string { return "?" },
	NumericCast: func(col string) string { return "CAST(" + col + " AS REAL)" },
	// SQLite has no DATE type; CAST(x AS DATE) would fall back to NUMERIC
	// affinity and mangle the string, so the date part is taken as text.
	DateCast: func(col string) string { return "substr(" + col + ", 1, 10)" },
}

func (p *SQLite) Search(filters schema.SearchFilters, offset, limit int) ([]schema.TelemetryData, int, error) {
	if limit <= 0 {
		limit = 50
	}
	where, args := filters.BuildWhere(sqliteDialect)
	whereSQL := ""
	if where != "" {
		whereSQL = " WHERE " + where
	}

	var total int
	if err := p.db.QueryRow(`SELECT COUNT(*) FROM speedtest_users`+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("sqlite search count: %w", err)
	}

	rows, err := p.db.Query(
		`SELECT id, timestamp, ip, ispinfo, extra, ua, lang, dl, ul, ping, jitter, log, uuid,
			COALESCE(grade_data,''), COALESCE(chart_data,''), COALESCE(latency_underload,''),
			COALESCE(ping_during_test,''), COALESCE(client_id,'')
		FROM speedtest_users`+whereSQL+` ORDER BY timestamp DESC, id DESC LIMIT ? OFFSET ?`,
		append(append([]interface{}{}, args...), limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("sqlite search: %w", err)
	}
	defer rows.Close()
	var records []schema.TelemetryData
	for rows.Next() {
		var record schema.TelemetryData
		if err := sqScan(rows, &record); err != nil {
			return nil, 0, fmt.Errorf("sqlite search scan row: %w", err)
		}
		records = append(records, record)
	}
	return records, total, nil
}

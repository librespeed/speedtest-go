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

	// Create table if not exists (matching the PHP SQLite auto-creation behavior)
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
		uuid TEXT
	);`
	if _, err := conn.Exec(stmt); err != nil {
		log.Fatalf("Failed to create speedtest_users table: %s", err)
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

	stmt := `INSERT INTO speedtest_users (ip, ispinfo, extra, ua, lang, dl, ul, ping, jitter, log, uuid) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	_, err = p.db.Exec(stmt, data.IPAddress, data.ISPInfo, data.Extra, data.UserAgent, data.Language, data.Download, data.Upload, data.Ping, data.Jitter, data.Log, data.UUID)
	return err
}

func (p *SQLite) FetchByUUID(uuid string) (*schema.TelemetryData, error) {
	var record schema.TelemetryData
	row := p.db.QueryRow(`SELECT * FROM speedtest_users WHERE uuid = ?`, uuid)
	if row != nil {
		var id int
		if err := row.Scan(&id, &record.Timestamp, &record.IPAddress, &record.ISPInfo, &record.Extra, &record.UserAgent, &record.Language, &record.Download, &record.Upload, &record.Ping, &record.Jitter, &record.Log, &record.UUID); err != nil {
			return nil, fmt.Errorf("sqlite fetch by uuid: %w", err)
		}
	}
	return &record, nil
}

func (p *SQLite) FetchLast100() ([]schema.TelemetryData, error) {
	var records []schema.TelemetryData
	rows, err := p.db.Query(`SELECT * FROM speedtest_users ORDER BY timestamp DESC LIMIT 100;`)
	if err != nil {
		return nil, fmt.Errorf("sqlite fetch last 100: %w", err)
	}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var record schema.TelemetryData
			var id int
			if err := rows.Scan(&id, &record.Timestamp, &record.IPAddress, &record.ISPInfo, &record.Extra, &record.UserAgent, &record.Language, &record.Download, &record.Upload, &record.Ping, &record.Jitter, &record.Log, &record.UUID); err != nil {
				return nil, fmt.Errorf("sqlite scan row: %w", err)
			}
			records = append(records, record)
		}
	}
	return records, nil
}

func (p *SQLite) FetchAll(offset, limit int) ([]schema.TelemetryData, error) {
	if limit <= 0 {
		limit = 50
	}
	var records []schema.TelemetryData
	rows, err := p.db.Query(`SELECT * FROM speedtest_users ORDER BY timestamp DESC LIMIT ? OFFSET ?;`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("sqlite fetch all: %w", err)
	}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var record schema.TelemetryData
			var id int
			if err := rows.Scan(&id, &record.Timestamp, &record.IPAddress, &record.ISPInfo, &record.Extra, &record.UserAgent, &record.Language, &record.Download, &record.Upload, &record.Ping, &record.Jitter, &record.Log, &record.UUID); err != nil {
				return nil, fmt.Errorf("sqlite scan row: %w", err)
			}
			records = append(records, record)
		}
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

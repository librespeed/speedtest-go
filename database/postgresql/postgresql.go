package postgresql

import (
	"database/sql"
	"fmt"

	"github.com/librespeed/speedtest-go/database/schema"

	_ "github.com/lib/pq"
	log "github.com/sirupsen/logrus"
)

const (
	connectionStringTemplate = `postgres://%s:%s@%s/%s?sslmode=disable`
)

type PostgreSQL struct {
	db *sql.DB
}

func Open(hostname, username, password, database string) *PostgreSQL {
	connStr := fmt.Sprintf(connectionStringTemplate, username, password, hostname, database)
	conn, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Cannot open PostgreSQL database: %s", err)
	}
	return &PostgreSQL{db: conn}
}

func pgScan(row interface{ Scan(...any) error }, record *schema.TelemetryData) error {
	var id string
	return row.Scan(&id, &record.Timestamp, &record.IPAddress, &record.ISPInfo, &record.Extra,
		&record.UserAgent, &record.Language, &record.Download, &record.Upload,
		&record.Ping, &record.Jitter, &record.Log, &record.UUID,
		&record.GradeData, &record.ChartData, &record.LatencyUnderload,
		&record.PingDuringTest, &record.ClientID)
}

func (p *PostgreSQL) Insert(data *schema.TelemetryData) error {
	_, err := p.db.Exec(
		`INSERT INTO speedtest_users
			(ip, ispinfo, extra, ua, lang, dl, ul, ping, jitter, log, uuid,
			 grade_data, chart_data, latency_underload, ping_during_test, client_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id;`,
		data.IPAddress, data.ISPInfo, data.Extra, data.UserAgent, data.Language,
		data.Download, data.Upload, data.Ping, data.Jitter, data.Log, data.UUID,
		data.GradeData, data.ChartData, data.LatencyUnderload, data.PingDuringTest, data.ClientID)
	return err
}

func (p *PostgreSQL) FetchByUUID(uuid string) (*schema.TelemetryData, error) {
	var record schema.TelemetryData
	row := p.db.QueryRow(
		`SELECT id, timestamp, ip, ispinfo, extra, ua, lang, dl, ul, ping, jitter, log, uuid,
			COALESCE(grade_data,''), COALESCE(chart_data,''), COALESCE(latency_underload,''),
			COALESCE(ping_during_test,''), COALESCE(client_id,'')
		FROM speedtest_users WHERE uuid = $1`, uuid)
	if err := pgScan(row, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func (p *PostgreSQL) FetchLast100() ([]schema.TelemetryData, error) {
	rows, err := p.db.Query(
		`SELECT id, timestamp, ip, ispinfo, extra, ua, lang, dl, ul, ping, jitter, log, uuid,
			COALESCE(grade_data,''), COALESCE(chart_data,''), COALESCE(latency_underload,''),
			COALESCE(ping_during_test,''), COALESCE(client_id,'')
		FROM speedtest_users ORDER BY timestamp DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []schema.TelemetryData
	for rows.Next() {
		var record schema.TelemetryData
		if err := pgScan(rows, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (p *PostgreSQL) FetchAll(offset, limit int) ([]schema.TelemetryData, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := p.db.Query(
		`SELECT id, timestamp, ip, ispinfo, extra, ua, lang, dl, ul, ping, jitter, log, uuid,
			COALESCE(grade_data,''), COALESCE(chart_data,''), COALESCE(latency_underload,''),
			COALESCE(ping_during_test,''), COALESCE(client_id,'')
		FROM speedtest_users ORDER BY timestamp DESC OFFSET $1 LIMIT $2`,
		offset, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []schema.TelemetryData
	for rows.Next() {
		var record schema.TelemetryData
		if err := pgScan(rows, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (p *PostgreSQL) Count() (int, error) {
	var count int
	err := p.db.QueryRow(`SELECT COUNT(*) FROM speedtest_users;`).Scan(&count)
	return count, err
}

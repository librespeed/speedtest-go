package mysql

import (
	"database/sql"
	"fmt"

	"github.com/librespeed/speedtest-go/database/schema"

	_ "github.com/go-sql-driver/mysql"
	log "github.com/sirupsen/logrus"
)

const (
	connectionStringTemplate = `%s:%s@%s/%s?parseTime=true`
)

type MySQL struct {
	db *sql.DB
}

func Open(hostname, username, password, database string) *MySQL {
	connStr := fmt.Sprintf(connectionStringTemplate, username, password, hostname, database)
	conn, err := sql.Open("mysql", connStr)
	if err != nil {
		log.Fatalf("Cannot open MySQL database: %s", err)
	}
	return &MySQL{db: conn}
}

func myScan(row interface{ Scan(...any) error }, record *schema.TelemetryData) error {
	var id string
	return row.Scan(&id, &record.Timestamp, &record.IPAddress, &record.ISPInfo, &record.Extra,
		&record.UserAgent, &record.Language, &record.Download, &record.Upload,
		&record.Ping, &record.Jitter, &record.Log, &record.UUID,
		&record.GradeData, &record.ChartData, &record.LatencyUnderload,
		&record.PingDuringTest, &record.ClientID)
}

func (p *MySQL) Insert(data *schema.TelemetryData) error {
	_, err := p.db.Exec(
		`INSERT INTO speedtest_users
			(ip, ispinfo, extra, ua, lang, dl, ul, ping, jitter, log, uuid,
			 grade_data, chart_data, latency_underload, ping_during_test, client_id)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		data.IPAddress, data.ISPInfo, data.Extra, data.UserAgent, data.Language,
		data.Download, data.Upload, data.Ping, data.Jitter, data.Log, data.UUID,
		data.GradeData, data.ChartData, data.LatencyUnderload, data.PingDuringTest, data.ClientID)
	return err
}

func (p *MySQL) FetchByUUID(uuid string) (*schema.TelemetryData, error) {
	var record schema.TelemetryData
	row := p.db.QueryRow(
		`SELECT id, timestamp, ip, ispinfo, extra, ua, lang, dl, ul, ping, jitter, log, uuid,
			COALESCE(grade_data,''), COALESCE(chart_data,''), COALESCE(latency_underload,''),
			COALESCE(ping_during_test,''), COALESCE(client_id,'')
		FROM speedtest_users WHERE uuid = ?`, uuid)
	if err := myScan(row, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func (p *MySQL) FetchLast100() ([]schema.TelemetryData, error) {
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
		if err := myScan(rows, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (p *MySQL) FetchAll(offset, limit int) ([]schema.TelemetryData, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := p.db.Query(
		`SELECT id, timestamp, ip, ispinfo, extra, ua, lang, dl, ul, ping, jitter, log, uuid,
			COALESCE(grade_data,''), COALESCE(chart_data,''), COALESCE(latency_underload,''),
			COALESCE(ping_during_test,''), COALESCE(client_id,'')
		FROM speedtest_users ORDER BY timestamp DESC LIMIT ? OFFSET ?`,
		limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []schema.TelemetryData
	for rows.Next() {
		var record schema.TelemetryData
		if err := myScan(rows, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (p *MySQL) Count() (int, error) {
	var count int
	err := p.db.QueryRow(`SELECT COUNT(*) FROM speedtest_users;`).Scan(&count)
	return count, err
}

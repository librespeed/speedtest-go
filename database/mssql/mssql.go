package mssql

import (
	"database/sql"
	"fmt"
	"net/url"

	"github.com/librespeed/speedtest-go/database/schema"

	_ "github.com/denisenkom/go-mssqldb"
	log "github.com/sirupsen/logrus"
)

type MSSQL struct {
	db *sql.DB
}

func Open(hostname, username, password, database, port string) *MSSQL {
	if port == "" {
		port = "1433"
	}

	query := url.Values{}
	query.Add("database", database)

	connStr := fmt.Sprintf("sqlserver://%s:%s@%s:%s?%s",
		url.QueryEscape(username),
		url.QueryEscape(password),
		hostname,
		port,
		query.Encode(),
	)

	conn, err := sql.Open("sqlserver", connStr)
	if err != nil {
		log.Fatalf("Cannot open MSSQL database: %s", err)
	}

	return &MSSQL{db: conn}
}

func msScan(row interface{ Scan(...any) error }, record *schema.TelemetryData) error {
	var id int64
	return row.Scan(&id, &record.Timestamp, &record.IPAddress, &record.ISPInfo, &record.Extra,
		&record.UserAgent, &record.Language, &record.Download, &record.Upload,
		&record.Ping, &record.Jitter, &record.Log, &record.UUID,
		&record.GradeData, &record.ChartData, &record.LatencyUnderload,
		&record.PingDuringTest, &record.ClientID)
}

func (p *MSSQL) Insert(data *schema.TelemetryData) error {
	_, err := p.db.Exec(
		`INSERT INTO speedtest_users
			(ip, ispinfo, extra, ua, lang, dl, ul, ping, jitter, log, uuid,
			 grade_data, chart_data, latency_underload, ping_during_test, client_id)
			VALUES (@p1,@p2,@p3,@p4,@p5,@p6,@p7,@p8,@p9,@p10,@p11,@p12,@p13,@p14,@p15,@p16)`,
		data.IPAddress, data.ISPInfo, data.Extra, data.UserAgent, data.Language,
		data.Download, data.Upload, data.Ping, data.Jitter, data.Log, data.UUID,
		data.GradeData, data.ChartData, data.LatencyUnderload, data.PingDuringTest, data.ClientID)
	return err
}

func (p *MSSQL) FetchByUUID(uuid string) (*schema.TelemetryData, error) {
	var record schema.TelemetryData
	row := p.db.QueryRow(
		`SELECT id, timestamp, ip, ispinfo, extra, ua, lang, dl, ul, ping, jitter, log, uuid,
			COALESCE(grade_data,''), COALESCE(chart_data,''), COALESCE(latency_underload,''),
			COALESCE(ping_during_test,''), COALESCE(client_id,'')
		FROM speedtest_users WHERE uuid = @p1`, uuid)
	if err := msScan(row, &record); err != nil {
		return nil, fmt.Errorf("mssql fetch by uuid: %w", err)
	}
	return &record, nil
}

func (p *MSSQL) FetchLast100() ([]schema.TelemetryData, error) {
	rows, err := p.db.Query(
		`SELECT TOP 100 id, timestamp, ip, ispinfo, extra, ua, lang, dl, ul, ping, jitter, log, uuid,
			COALESCE(grade_data,''), COALESCE(chart_data,''), COALESCE(latency_underload,''),
			COALESCE(ping_during_test,''), COALESCE(client_id,'')
		FROM speedtest_users ORDER BY timestamp DESC`)
	if err != nil {
		return nil, fmt.Errorf("mssql fetch last 100: %w", err)
	}
	defer rows.Close()
	var records []schema.TelemetryData
	for rows.Next() {
		var record schema.TelemetryData
		if err := msScan(rows, &record); err != nil {
			return nil, fmt.Errorf("mssql scan row: %w", err)
		}
		records = append(records, record)
	}
	return records, nil
}

func (p *MSSQL) FetchAll(offset, limit int) ([]schema.TelemetryData, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := p.db.Query(
		`SELECT id, timestamp, ip, ispinfo, extra, ua, lang, dl, ul, ping, jitter, log, uuid,
			COALESCE(grade_data,''), COALESCE(chart_data,''), COALESCE(latency_underload,''),
			COALESCE(ping_during_test,''), COALESCE(client_id,'')
		FROM speedtest_users ORDER BY timestamp DESC OFFSET @p1 ROWS FETCH NEXT @p2 ROWS ONLY`,
		offset, limit)
	if err != nil {
		return nil, fmt.Errorf("mssql fetch all: %w", err)
	}
	defer rows.Close()
	var records []schema.TelemetryData
	for rows.Next() {
		var record schema.TelemetryData
		if err := msScan(rows, &record); err != nil {
			return nil, fmt.Errorf("mssql scan row: %w", err)
		}
		records = append(records, record)
	}
	return records, nil
}

func (p *MSSQL) Count() (int, error) {
	var count int
	err := p.db.QueryRow(`SELECT COUNT(*) FROM speedtest_users;`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("mssql count: %w", err)
	}
	return count, nil
}

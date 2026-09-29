package memory

import (
	"errors"
	"sync"
	"time"

	"github.com/librespeed/speedtest-go/database/schema"
)

const (
	// just enough records to return for FetchLast100
	maxRecords = 100
)

type Memory struct {
	lock    sync.RWMutex
	records []schema.TelemetryData
}

func Open(_ string) *Memory {
	return &Memory{}
}

func (mem *Memory) Insert(data *schema.TelemetryData) error {
	mem.lock.Lock()
	defer mem.lock.Unlock()
	data.Timestamp = time.Now()
	mem.records = append(mem.records, *data)
	if len(mem.records) > maxRecords {
		mem.records = mem.records[len(mem.records)-maxRecords:]
	}
	return nil
}

func (mem *Memory) FetchByUUID(uuid string) (*schema.TelemetryData, error) {
	mem.lock.RLock()
	defer mem.lock.RUnlock()
	for _, record := range mem.records {
		if record.UUID == uuid {
			return &record, nil
		}
	}
	return nil, errors.New("record not found")
}

func (mem *Memory) FetchLast100() ([]schema.TelemetryData, error) {
	mem.lock.RLock()
	defer mem.lock.RUnlock()
	return mem.records, nil
}

func (mem *Memory) FetchAll(offset, limit int) ([]schema.TelemetryData, error) {
	if limit <= 0 {
		limit = 50
	}
	mem.lock.RLock()
	defer mem.lock.RUnlock()

	n := len(mem.records)
	// records are stored oldest→first; we want newest→first
	if offset >= n {
		return nil, nil
	}
	end := n - offset
	start := end - limit
	if start < 0 {
		start = 0
	}
	out := make([]schema.TelemetryData, 0, end-start)
	for i := end - 1; i >= start; i-- {
		out = append(out, mem.records[i])
	}
	return out, nil
}

func (mem *Memory) Count() (int, error) {
	mem.lock.RLock()
	defer mem.lock.RUnlock()
	return len(mem.records), nil
}

// Search returns one page of records matching filters (newest first), plus
// the total match count. This backend already keeps everything in RAM (it's
// capped at maxRecords), so there's no separate storage layer to push the
// filtering into — this just applies it without an extra full-slice copy.
func (mem *Memory) Search(filters schema.SearchFilters, offset, limit int) ([]schema.TelemetryData, int, error) {
	if limit <= 0 {
		limit = 50
	}
	mem.lock.RLock()
	defer mem.lock.RUnlock()

	var records []schema.TelemetryData
	matched := 0
	for i := len(mem.records) - 1; i >= 0; i-- {
		if !filters.Matches(mem.records[i]) {
			continue
		}
		if matched >= offset && len(records) < limit {
			records = append(records, mem.records[i])
		}
		matched++
	}
	return records, matched, nil
}

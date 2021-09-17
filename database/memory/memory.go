package memory

import (
	"errors"
	"sync"
	"time"

	"github.com/librespeed/speedtest/database/schema"
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

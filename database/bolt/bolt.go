package bolt

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/librespeed/speedtest-go/database/schema"

	log "github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
)

const (
	bucketName = `speedtest`
)

type Bolt struct {
	db *bbolt.DB
}

func Open(databaseFile string) *Bolt {
	db, err := bbolt.Open(databaseFile, 0666, nil)
	if err != nil {
		log.Fatalf("Cannot open BoltDB database file: %s", err)
	}
	return &Bolt{db: db}
}

func (p *Bolt) Insert(data *schema.TelemetryData) error {
	return p.db.Update(func(tx *bbolt.Tx) error {
		data.Timestamp = time.Now()
		b, _ := json.Marshal(data)
		bucket, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		if err != nil {
			return err
		}
		return bucket.Put([]byte(data.UUID), b)
	})
}

func (p *Bolt) FetchByUUID(uuid string) (*schema.TelemetryData, error) {
	var record schema.TelemetryData
	err := p.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return errors.New("data bucket doesn't exist yet")
		}
		b := bucket.Get([]byte(uuid))
		return json.Unmarshal(b, &record)
	})
	return &record, err
}

func (p *Bolt) FetchLast100() ([]schema.TelemetryData, error) {
	var records []schema.TelemetryData
	err := p.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return errors.New("data bucket doesn't exist yet")
		}

		cursor := bucket.Cursor()
		_, b := cursor.Last()

		for len(records) < 100 {
			// IMPORTANT: declare a fresh record on every iteration.
			// json.Unmarshal does NOT reset fields that are absent from the JSON,
			// so reusing one struct would let values (e.g. ClientID, GradeData)
			// leak from an earlier row into a later one.
			var record schema.TelemetryData
			if err := json.Unmarshal(b, &record); err != nil {
				return err
			}
			records = append(records, record)

			_, b = cursor.Prev()
			if b == nil {
				break
			}
		}

		return nil
	})
	return records, err
}

// FetchAll returns one page of records (most recent first), skipping `offset`
// records. ULID keys are lexicographically ordered by time, so descending key
// order == newest first.
func (p *Bolt) FetchAll(offset, limit int) ([]schema.TelemetryData, error) {
	if limit <= 0 {
		limit = 50
	}
	var records []schema.TelemetryData
	err := p.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return errors.New("data bucket doesn't exist yet")
		}

		skipped, taken := 0, 0
		cursor := bucket.Cursor()
		for k, b := cursor.Last(); k != nil; k, b = cursor.Prev() {
			if skipped < offset {
				skipped++
				continue
			}
			if taken >= limit {
				break
			}
			var record schema.TelemetryData // fresh each iteration (see FetchLast100)
			if err := json.Unmarshal(b, &record); err != nil {
				return err
			}
			records = append(records, record)
			taken++
		}
		return nil
	})
	return records, err
}

// Count returns the total number of stored records.
func (p *Bolt) Count() (int, error) {
	var count int
	err := p.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return nil // no bucket yet → 0 records
		}
		count = bucket.Stats().KeyN
		return nil
	})
	return count, err
}

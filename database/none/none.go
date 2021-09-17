package none

import (
	"github.com/librespeed/speedtest/database/schema"
)

type None struct{}

func Open(_ string) *None {
	return &None{}
}

func (n *None) Insert(_ *schema.TelemetryData) error {
	return nil
}

func (n *None) FetchByUUID(_ string) (*schema.TelemetryData, error) {
	return &schema.TelemetryData{}, nil
}

func (n *None) FetchLast100() ([]schema.TelemetryData, error) {
	return []schema.TelemetryData{}, nil
}

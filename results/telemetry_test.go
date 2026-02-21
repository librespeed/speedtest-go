package results

import (
	"testing"
)

func TestIPRedaction(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantOut  string
		regex    func(string, string) string
		redactTo string
	}{
		{
			name:     "IPv4 in ispInfo is redacted",
			input:    `{"ip":"203.0.113.42","org":"AS12345 Example ISP"}`,
			wantOut:  `{"ip":"0.0.0.0","org":"AS12345 Example ISP"}`,
			regex:    ipv4Regex.ReplaceAllString,
			redactTo: "0.0.0.0",
		},
		{
			name:     "IPv6 in ispInfo is redacted",
			input:    `{"ip":"2001:0db8:85a3:0000:0000:8a2e:0370:7334","org":"AS12345 Example ISP"}`,
			wantOut:  `{"ip":"0.0.0.0","org":"AS12345 Example ISP"}`,
			regex:    ipv6Regex.ReplaceAllString,
			redactTo: "0.0.0.0",
		},
		{
			name:     "hostname in ispInfo is redacted",
			input:    `{"ip":"0.0.0.0","hostname":"client.example.com","org":"AS12345 Example ISP"}`,
			wantOut:  `{"ip":"0.0.0.0","hostname":"REDACTED","org":"AS12345 Example ISP"}`,
			regex:    hostnameRegex.ReplaceAllString,
			redactTo: `"hostname":"REDACTED"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.regex(tt.input, tt.redactTo)
			if got != tt.wantOut {
				t.Errorf("\ngot:  %s\nwant: %s", got, tt.wantOut)
			}
		})
	}
}

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
			name:     "multiple IPv4 addresses in logs are all redacted",
			input:    `connected from 203.0.113.42, forwarded for 198.51.100.7`,
			wantOut:  `connected from 0.0.0.0, forwarded for 0.0.0.0`,
			regex:    ipv4Regex.ReplaceAllString,
			redactTo: "0.0.0.0",
		},
		{
			name:     "empty string is handled safely by IPv4 regex",
			input:    "",
			wantOut:  "",
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
			name:     "empty string is handled safely by IPv6 regex",
			input:    "",
			wantOut:  "",
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
		{
			name:     "empty string is handled safely by hostname regex",
			input:    "",
			wantOut:  "",
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

func TestIPRedactionChain(t *testing.T) {
	// Mirrors the full redaction block in Record() — all six replacements
	// applied sequentially to the same ispInfo and logs strings.
	ispInfo := `{"ip":"203.0.113.42","hostname":"client.example.com","org":"AS12345 Example ISP"}`
	logs := `connected from 203.0.113.42 and 198.51.100.7`

	ispInfo = ipv4Regex.ReplaceAllString(ispInfo, "0.0.0.0")
	logs = ipv4Regex.ReplaceAllString(logs, "0.0.0.0")
	ispInfo = ipv6Regex.ReplaceAllString(ispInfo, "0.0.0.0")
	logs = ipv6Regex.ReplaceAllString(logs, "0.0.0.0")
	ispInfo = hostnameRegex.ReplaceAllString(ispInfo, `"hostname":"REDACTED"`)
	logs = hostnameRegex.ReplaceAllString(logs, `"hostname":"REDACTED"`)

	wantISP := `{"ip":"0.0.0.0","hostname":"REDACTED","org":"AS12345 Example ISP"}`
	wantLogs := `connected from 0.0.0.0 and 0.0.0.0`

	if ispInfo != wantISP {
		t.Errorf("ispInfo\ngot:  %s\nwant: %s", ispInfo, wantISP)
	}
	if logs != wantLogs {
		t.Errorf("logs\ngot:  %s\nwant: %s", logs, wantLogs)
	}
}

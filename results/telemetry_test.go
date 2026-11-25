package results

import (
	"encoding/json"
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
			wantOut:  `{"ip":"::","org":"AS12345 Example ISP"}`,
			regex:    ipv6Regex.ReplaceAllString,
			redactTo: "::",
		},
		{
			name:     "empty string is handled safely by IPv6 regex",
			input:    "",
			wantOut:  "",
			regex:    ipv6Regex.ReplaceAllString,
			redactTo: "::",
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
	tests := []struct {
		name     string
		ispInfo  string
		logs     string
		wantISP  string
		wantLogs string
	}{
		{
			name:     "IPv4 client: addresses and hostname redacted",
			ispInfo:  `{"ip":"203.0.113.42","hostname":"client.example.com","org":"AS12345 Example ISP"}`,
			logs:     `connected from 203.0.113.42 and 198.51.100.7`,
			wantISP:  `{"ip":"0.0.0.0","hostname":"REDACTED","org":"AS12345 Example ISP"}`,
			wantLogs: `connected from 0.0.0.0 and 0.0.0.0`,
		},
		{
			name:     "IPv6 client: redacted as :: to preserve address family for debugging",
			ispInfo:  `{"ip":"2001:0db8:85a3:0000:0000:8a2e:0370:7334","hostname":"client.example.com","org":"AS12345 Example ISP"}`,
			logs:     `connected from 2001:0db8:85a3:0000:0000:8a2e:0370:7334`,
			wantISP:  `{"ip":"::","hostname":"REDACTED","org":"AS12345 Example ISP"}`,
			wantLogs: `connected from ::`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ispInfo := tt.ispInfo
			logs := tt.logs

			// Mirror the full redaction block in Record()
			ispInfo = ipv4Regex.ReplaceAllString(ispInfo, "0.0.0.0")
			logs = ipv4Regex.ReplaceAllString(logs, "0.0.0.0")
			ispInfo = ipv6Regex.ReplaceAllString(ispInfo, "::")
			logs = ipv6Regex.ReplaceAllString(logs, "::")
			ispInfo = hostnameRegex.ReplaceAllString(ispInfo, `"hostname":"REDACTED"`)
			logs = hostnameRegex.ReplaceAllString(logs, `"hostname":"REDACTED"`)

			if ispInfo != tt.wantISP {
				t.Errorf("ispInfo\ngot:  %s\nwant: %s", ispInfo, tt.wantISP)
			}
			if logs != tt.wantLogs {
				t.Errorf("logs\ngot:  %s\nwant: %s", logs, tt.wantLogs)
			}
		})
	}
}

func TestLegacyISPPayloadCompatibility(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name:    "legacy object",
			payload: `{"processedString":"128.94.2.134 - Microsoft Corporation, US","rawIspInfo":{"ip":"128.94.2.134","city":"Boydton","region":"Virginia","country":"US","loc":"36.6676,-78.3875","org":"AS8075 Microsoft Corporation"}}`,
		},
		{
			name:    "empty string raw payload",
			payload: `{"processedString":"128.94.2.134","rawIspInfo":"","location":null}`,
		},
		{
			name:    "null raw payload",
			payload: `{"processedString":"128.94.2.134","rawIspInfo":null}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var result Result
			err := json.Unmarshal([]byte(test.payload), &result)
			if err != nil {
				t.Fatalf("legacy payload should remain decodable: %v", err)
			}
			if result.ProcessedString == "" {
				t.Fatal("processedString was lost")
			}
		})
	}

	location := &Location{
		City:        "Boydton",
		Region:      "Virginia",
		Country:     "US",
		CountryName: "United States",
		Postal:      "23917",
		Timezone:    "America/New_York",
		Source:      "ipinfo",
	}
	latitude := 36.6676
	longitude := -78.3875
	location.Latitude = &latitude
	location.Longitude = &longitude

	payload, err := json.Marshal(Result{
		ProcessedString: "128.94.2.134 - Microsoft Corporation, US",
		RawISPInfo: IPInfoResponse{
			IP:           "128.94.2.134",
			City:         "Boydton",
			Region:       "Virginia",
			Country:      "US",
			Location:     "36.6676,-78.3875",
			Organization: "AS8075 Microsoft Corporation",
			Postal:       "23917",
			Timezone:     "America/New_York",
		},
		Location: location,
	})
	if err != nil {
		t.Fatalf("marshal structured payload: %v", err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode payload map: %v", err)
	}
	if _, ok := decoded["processedString"]; !ok {
		t.Fatal("processedString missing from structured payload")
	}
	if _, ok := decoded["rawIspInfo"]; !ok {
		t.Fatal("rawIspInfo missing from structured payload")
	}
	if _, ok := decoded["location"]; !ok {
		t.Fatal("location missing from structured payload")
	}

	var locationObject map[string]json.RawMessage
	if err := json.Unmarshal(decoded["location"], &locationObject); err != nil {
		t.Fatalf("decode location object: %v", err)
	}
	for _, allowed := range []string{
		"city", "region", "country", "countryName", "postal", "timezone",
		"latitude", "longitude", "source",
	} {
		delete(locationObject, allowed)
	}
	if len(locationObject) != 0 {
		t.Fatalf("location object contains unexpected fields: %#v", locationObject)
	}

	legacyOnlyPayload, err := json.Marshal(Result{
		ProcessedString: "128.94.2.134",
		RawISPInfo:      IPInfoResponse{},
	})
	if err != nil {
		t.Fatalf("marshal legacy-only payload: %v", err)
	}
	var legacyOnly map[string]json.RawMessage
	if err := json.Unmarshal(legacyOnlyPayload, &legacyOnly); err != nil {
		t.Fatalf("decode legacy-only payload: %v", err)
	}
	if _, ok := legacyOnly["location"]; ok {
		t.Fatal("nil location should be omitted")
	}
}

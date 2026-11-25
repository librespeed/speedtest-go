package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/librespeed/speedtest-go/config"
	"github.com/librespeed/speedtest-go/results"
)

func TestGetIPResponseIncludesLegacyAndStructuredLocation(t *testing.T) {
	config.Load("../settings.toml")

	oldIPLookup := ipInfoLookup
	oldGeoLookup := geoIPLookup
	ipInfoLookup = func(string) results.IPInfoResponse {
		return results.IPInfoResponse{
			IP:           "128.94.2.134",
			City:         "Boydton",
			Region:       "Virginia",
			Country:      "US",
			CountryName:  "United States",
			Location:     "36.6676,-78.3875",
			Organization: "AS8075 Microsoft Corporation",
			Postal:       "23917",
			Timezone:     "America/New_York",
		}
	}
	geoIPLookup = func(string) *GeoIPData {
		return nil
	}
	t.Cleanup(func() {
		ipInfoLookup = oldIPLookup
		geoIPLookup = oldGeoLookup
		config.Load("../settings.toml")
	})

	request := httptest.NewRequest(http.MethodGet, "/backend/getIP?isp=true&distance=km", nil)
	request.RemoteAddr = "128.94.2.134:12345"
	recorder := httptest.NewRecorder()

	getIP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}

	var response results.Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode getIP response: %v", err)
	}
	if response.ProcessedString == "" {
		t.Fatal("missing processedString")
	}
	if response.ProcessedString == "128.94.2.134" {
		t.Fatal("ISP lookup did not produce the legacy processedString")
	}
	if response.RawISPInfo.Organization != "AS8075 Microsoft Corporation" {
		t.Fatalf("unexpected rawIspInfo organization: %q", response.RawISPInfo.Organization)
	}
	if response.Location == nil {
		t.Fatal("missing top-level location")
	}
	if response.Location.City != "Boydton" ||
		response.Location.Region != "Virginia" ||
		response.Location.Country != "US" ||
		response.Location.CountryName != "United States" ||
		response.Location.Source != "ipinfo" {
		t.Fatalf("unexpected location payload: %#v", response.Location)
	}
	if response.Location.Latitude == nil || response.Location.Longitude == nil {
		t.Fatalf("missing location coordinates: %#v", response.Location)
	}

	legacyPayload, err := json.Marshal(map[string]interface{}{
		"processedString": response.ProcessedString,
		"rawIspInfo":      response.RawISPInfo,
	})
	if err != nil {
		t.Fatalf("marshal legacy fields: %v", err)
	}
	var legacyResult results.Result
	if err := json.Unmarshal(legacyPayload, &legacyResult); err != nil {
		t.Fatalf("legacy fields should remain decodable: %v", err)
	}
	if legacyResult.ProcessedString != response.ProcessedString {
		t.Fatal("processedString compatibility changed")
	}
}

func TestGetIPMalformedLocationHasNoEmptyDistanceSuffix(t *testing.T) {
	config.Load("../settings.toml")

	oldIPLookup := ipInfoLookup
	oldGeoLookup := geoIPLookup
	ipInfoLookup = func(string) results.IPInfoResponse {
		return results.IPInfoResponse{
			IP:           "128.94.2.134",
			City:         "Boydton",
			Region:       "Virginia",
			Country:      "US",
			Location:     "91,-78.3875",
			Organization: "AS8075 Microsoft Corporation",
		}
	}
	geoIPLookup = func(string) *GeoIPData {
		return nil
	}
	t.Cleanup(func() {
		ipInfoLookup = oldIPLookup
		geoIPLookup = oldGeoLookup
		config.Load("../settings.toml")
	})

	request := httptest.NewRequest(http.MethodGet, "/getIP?isp=true&distance=km", nil)
	request.RemoteAddr = "128.94.2.134:12345"
	recorder := httptest.NewRecorder()
	getIP(recorder, request)

	var response results.Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode getIP response: %v", err)
	}
	if strings.Contains(response.ProcessedString, "()") {
		t.Fatalf("malformed location created an empty distance suffix: %q", response.ProcessedString)
	}
	if !strings.HasSuffix(response.ProcessedString, "Microsoft Corporation, US") {
		t.Fatalf("unexpected malformed-location processedString: %q", response.ProcessedString)
	}
	if response.Location != nil {
		if response.Location.Latitude != nil || response.Location.Longitude != nil {
			t.Fatalf("malformed coordinates should not be emitted: %#v", response.Location)
		}
	}
}

func TestGetIPDisableIPInfoSkipsLookup(t *testing.T) {
	config.Load("../settings.toml")

	// Use config.Load with a temporary settings file to exercise the real
	// disable_ipinfo configuration path.
	tempFile := t.TempDir() + "/settings.toml"
	if err := os.WriteFile(tempFile, []byte("disable_ipinfo=true\n"), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	config.Load(tempFile)
	t.Cleanup(func() {
		config.Load("../settings.toml")
	})

	oldIPLookup := ipInfoLookup
	called := false
	ipInfoLookup = func(string) results.IPInfoResponse {
		called = true
		return results.IPInfoResponse{}
	}
	t.Cleanup(func() {
		ipInfoLookup = oldIPLookup
	})

	request := httptest.NewRequest(http.MethodGet, "/getIP?isp=true", nil)
	request.RemoteAddr = "128.94.2.134:12345"
	recorder := httptest.NewRecorder()
	getIP(recorder, request)

	if called {
		t.Fatal("IP lookup was called while disable_ipinfo was enabled")
	}
	var response results.Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode disabled getIP response: %v", err)
	}
	if response.ProcessedString != "128.94.2.134" {
		t.Fatalf("unexpected disabled processedString: %q", response.ProcessedString)
	}
	if response.Location != nil {
		t.Fatalf("location should be omitted when disabled: %#v", response.Location)
	}
}

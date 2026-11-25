package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/librespeed/speedtest-go/config"
	"github.com/librespeed/speedtest-go/results"
)

func TestIPInfoURLSupportsUnauthenticatedAndAPIKeyRequests(t *testing.T) {
	oldBaseURL := ipInfoBaseURL
	ipInfoBaseURL = "https://ipinfo.io"
	t.Cleanup(func() {
		ipInfoBaseURL = oldBaseURL
	})

	unauthenticated := getIPInfoURLWithKey("128.94.2.134", "")
	if unauthenticated != "https://ipinfo.io/128.94.2.134/json" {
		t.Fatalf("unexpected unauthenticated URL: %s", unauthenticated)
	}

	withKey := getIPInfoURLWithKey("128.94.2.134", "test-token")
	parsed, err := url.Parse(withKey)
	if err != nil {
		t.Fatalf("parse API-key URL: %v", err)
	}
	if parsed.Path != "/128.94.2.134/json" {
		t.Fatalf("unexpected API-key path: %s", parsed.Path)
	}
	if parsed.Query().Get("token") != "test-token" {
		t.Fatalf("API-key token missing from URL: %s", withKey)
	}
}

func TestGetIPInfoAPIResponseAndLocation(t *testing.T) {
	config.Load("../settings.toml")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/128.94.2.134/json" {
			t.Errorf("unexpected request path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("token") != "" {
			t.Errorf("unexpected API token on unauthenticated request")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"ip":"128.94.2.134",
			"city":"Boydton",
			"region":"Virginia",
			"country":"US",
			"loc":"36.6676,-78.3875",
			"org":"AS8075 Microsoft Corporation",
			"postal":"23917",
			"timezone":"America/New_York"
		}`))
	}))
	defer server.Close()

	oldBaseURL := ipInfoBaseURL
	oldClient := ipInfoHTTPClient
	ipInfoBaseURL = server.URL
	ipInfoHTTPClient = server.Client()
	t.Cleanup(func() {
		ipInfoBaseURL = oldBaseURL
		ipInfoHTTPClient = oldClient
		config.Load("../settings.toml")
	})

	info := getIPInfo("128.94.2.134")
	if info.Organization != "AS8075 Microsoft Corporation" {
		t.Fatalf("unexpected organization: %q", info.Organization)
	}
	if info.Country != "US" {
		t.Fatalf("unexpected country: %q", info.Country)
	}
	if info.CountryName != "United States" {
		t.Fatalf("unexpected country name: %q", info.CountryName)
	}

	location := locationFromIPInfo(info)
	if location == nil {
		t.Fatal("expected structured location")
	}
	if location.City != "Boydton" || location.Region != "Virginia" {
		t.Fatalf("unexpected city/region: %#v", location)
	}
	if location.Country != "US" || location.CountryName != "United States" {
		t.Fatalf("unexpected country fields: %#v", location)
	}
	if location.Postal != "23917" || location.Timezone != "America/New_York" {
		t.Fatalf("unexpected postal/timezone: %#v", location)
	}
	if location.Latitude == nil || location.Longitude == nil {
		t.Fatalf("missing coordinates: %#v", location)
	}
	if *location.Latitude != 36.6676 || *location.Longitude != -78.3875 {
		t.Fatalf("unexpected coordinates: %.6f, %.6f", *location.Latitude, *location.Longitude)
	}
	if location.Source != "ipinfo" {
		t.Fatalf("unexpected source: %q", location.Source)
	}
}

func TestNormalizeGeoIPMaxMindCityFormat(t *testing.T) {
	geo := normalizeGeoIPResult(map[string]interface{}{
		"autonomous_system": map[string]interface{}{
			"organization": "Microsoft Corporation",
		},
		"country": map[string]interface{}{
			"iso_code": "US",
			"names": map[string]interface{}{
				"en": "United States",
			},
		},
		"city": map[string]interface{}{
			"names": map[string]interface{}{
				"en": "Boydton",
			},
		},
		"subdivisions": []interface{}{
			map[string]interface{}{
				"names": map[string]interface{}{
					"en": "Virginia",
				},
			},
		},
		"postal": map[string]interface{}{
			"code": "23917",
		},
		"location": map[string]interface{}{
			"latitude":  36.6676,
			"longitude": -78.3875,
			"time_zone": "America/New_York",
		},
	})
	if geo == nil {
		t.Fatal("expected normalized GeoIP data")
	}
	if geo.ASName != "Microsoft Corporation" {
		t.Fatalf("unexpected AS name: %q", geo.ASName)
	}
	if geo.City != "Boydton" || geo.Region != "Virginia" {
		t.Fatalf("unexpected city/region: %#v", geo)
	}
	if geo.Country != "US" || geo.CountryName != "United States" {
		t.Fatalf("unexpected country fields: %#v", geo)
	}
	if geo.Postal != "23917" || geo.Timezone != "America/New_York" {
		t.Fatalf("unexpected postal/timezone: %#v", geo)
	}
	if geo.Latitude == nil || geo.Longitude == nil {
		t.Fatalf("missing coordinates: %#v", geo)
	}
	if geo.Location != "36.667600,-78.387500" {
		t.Fatalf("unexpected normalized loc: %q", geo.Location)
	}
}

func TestOfflineGeoIPFallback(t *testing.T) {
	config.Load("../settings.toml")

	oldIPLookup := ipInfoLookup
	oldGeoLookup := geoIPLookup
	ipInfoLookup = func(string) results.IPInfoResponse {
		return results.IPInfoResponse{}
	}
	geoIPLookup = func(string) *GeoIPData {
		latitude := 36.6676
		longitude := -78.3875
		return &GeoIPData{
			ASName:      "Microsoft Corporation",
			CountryName: "United States",
			City:        "Boydton",
			Region:      "Virginia",
			Country:     "US",
			Postal:      "23917",
			Timezone:    "America/New_York",
			Latitude:    &latitude,
			Longitude:   &longitude,
			Location:    "36.667600,-78.387500",
		}
	}
	t.Cleanup(func() {
		ipInfoLookup = oldIPLookup
		geoIPLookup = oldGeoLookup
		config.Load("../settings.toml")
	})

	info, location := getIPInfoAndLocationByPriority("128.94.2.134")
	if info.IP != "128.94.2.134" {
		t.Fatalf("unexpected fallback IP: %q", info.IP)
	}
	if info.Organization != "Microsoft Corporation" || info.Country != "US" {
		t.Fatalf("unexpected fallback ISP/country: %#v", info)
	}
	if info.City != "Boydton" || info.Region != "Virginia" {
		t.Fatalf("unexpected fallback city/region: %#v", info)
	}
	if location == nil || location.Source != "geoip" {
		t.Fatalf("expected GeoIP location source: %#v", location)
	}
	if location.Latitude == nil || location.Longitude == nil {
		t.Fatalf("missing fallback coordinates: %#v", location)
	}
}

func TestMalformedCoordinatesAreRejected(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "valid negative", value: "-23.5504,-46.6339", valid: true},
		{name: "latitude too large", value: "91,-46.6339", valid: false},
		{name: "longitude too large", value: "-23.5504,-181", valid: false},
		{name: "NaN", value: "NaN,-46.6339", valid: false},
		{name: "missing longitude", value: "-23.5504", valid: false},
		{name: "extra component", value: "-23.5504,-46.6339,12", valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, valid := parseValidatedCoordinates(test.value)
			if valid != test.valid {
				t.Fatalf("parseValidatedCoordinates(%q) valid=%v, want %v", test.value, valid, test.valid)
			}
		})
	}

	location := locationFromIPInfo(results.IPInfoResponse{
		Location: "91,-46.6339",
	})
	if location != nil {
		t.Fatalf("malformed loc should not produce a location object: %#v", location)
	}

	latitude := 91.0
	longitude := -46.6339
	geoLocation := locationFromGeoIPData(&GeoIPData{
		Latitude:  &latitude,
		Longitude: &longitude,
	})
	if geoLocation != nil {
		t.Fatalf("out-of-range GeoIP coordinates should be omitted: %#v", geoLocation)
	}
}

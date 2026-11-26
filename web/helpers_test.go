package web

import (
	"bytes"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/librespeed/speedtest-go/config"
	"github.com/librespeed/speedtest-go/results"
	log "github.com/sirupsen/logrus"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

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

func TestGetIPInfoTransportErrorDoesNotLeakAPIKey(t *testing.T) {
	config.Load("../settings.toml")
	conf := config.LoadedConfig()
	oldKey := conf.IPInfoAPIKey
	conf.IPInfoAPIKey = "SECRETTOKEN"

	oldBaseURL := ipInfoBaseURL
	oldClient := ipInfoHTTPClient
	oldOutput := log.StandardLogger().Out
	var logOutput bytes.Buffer
	ipInfoFailureMu.Lock()
	oldCooldown := ipInfoFailureCooldownUntil
	ipInfoFailureCooldownUntil = time.Time{}
	ipInfoFailureMu.Unlock()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("open closed-port probe: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close closed-port probe: %v", err)
	}

	ipInfoBaseURL = "http://" + address
	ipInfoHTTPClient = &http.Client{Timeout: 250 * time.Millisecond}
	log.SetOutput(&logOutput)
	t.Cleanup(func() {
		conf.IPInfoAPIKey = oldKey
		ipInfoBaseURL = oldBaseURL
		ipInfoHTTPClient = oldClient
		log.SetOutput(oldOutput)
		ipInfoFailureMu.Lock()
		ipInfoFailureCooldownUntil = oldCooldown
		ipInfoFailureMu.Unlock()
		config.Load("../settings.toml")
	})

	_ = getIPInfo("1.2.3.4")

	logText := logOutput.String()
	if strings.Contains(logText, "SECRETTOKEN") {
		t.Fatalf("transport log leaked API key: %q", logText)
	}
	if logText == "" {
		t.Fatal("expected transport error log")
	}
}

func TestGetIPInfoFailureCooldown(t *testing.T) {
	config.Load("../settings.toml")

	oldClient := ipInfoHTTPClient
	oldBaseURL := ipInfoBaseURL
	calls := 0
	ipInfoBaseURL = "https://127.0.0.1"
	ipInfoHTTPClient = &http.Client{
		Timeout: 250 * time.Millisecond,
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return nil, errors.New("provider unavailable")
		}),
	}
	ipInfoFailureMu.Lock()
	oldCooldown := ipInfoFailureCooldownUntil
	ipInfoFailureCooldownUntil = time.Time{}
	ipInfoFailureMu.Unlock()
	t.Cleanup(func() {
		ipInfoHTTPClient = oldClient
		ipInfoBaseURL = oldBaseURL
		ipInfoFailureMu.Lock()
		ipInfoFailureCooldownUntil = oldCooldown
		ipInfoFailureMu.Unlock()
		config.Load("../settings.toml")
	})

	_ = getIPInfo("1.2.3.4")
	_ = getIPInfo("1.2.3.4")
	if calls != 1 {
		t.Fatalf("provider should be cooled down after first failure, calls=%d", calls)
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
		"autonomous_system_organization": "Microsoft Corporation",
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
	if geo.Location != "36.668,-78.388" {
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
			Location:    "36.668,-78.388",
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

func TestCountryConflictKeepsGeoIPPrimary(t *testing.T) {
	config.Load("../settings.toml")

	oldIPLookup := ipInfoLookup
	oldGeoLookup := geoIPLookup
	ipInfoLookup = func(string) results.IPInfoResponse {
		return results.IPInfoResponse{
			IP:       "128.94.2.134",
			City:     "Washington",
			Region:   "District of Columbia",
			Country:  "US",
			Location: "38.9072,-77.0369",
		}
	}
	geoIPLookup = func(string) *GeoIPData {
		latitude := 43.6532
		longitude := -79.3832
		return &GeoIPData{
			ASName:      "Example ISP",
			CountryName: "Canada",
			City:        "Toronto",
			Region:      "Ontario",
			Country:     "CA",
			Postal:      "M5V",
			Timezone:    "America/Toronto",
			Latitude:    &latitude,
			Longitude:   &longitude,
			Location:    "43.653,-79.383",
		}
	}
	t.Cleanup(func() {
		ipInfoLookup = oldIPLookup
		geoIPLookup = oldGeoLookup
		config.Load("../settings.toml")
	})

	info, location := getIPInfoAndLocationByPriority("128.94.2.134")
	if info.Organization != "Example ISP" {
		t.Fatalf("organization fallback should survive country conflict: %q", info.Organization)
	}
	if info.Country != "CA" {
		t.Fatalf("GeoIP country should remain primary: %q", info.Country)
	}
	if info.City != "Toronto" || info.Region != "Ontario" {
		t.Fatalf("GeoIP city/region should remain primary: %#v", info)
	}
	if info.Location != "43.653,-79.383" {
		t.Fatalf("GeoIP coordinates should remain primary: %q", info.Location)
	}
	if location == nil || location.Source != "geoip" {
		t.Fatalf("conflicting API should not replace GeoIP provenance: %#v", location)
	}
	if location.City != "Toronto" || location.Country != "CA" {
		t.Fatalf("unexpected conflict location: %#v", location)
	}
	if location.Latitude == nil || *location.Latitude != 43.6532 ||
		location.Longitude == nil || *location.Longitude != -79.3832 {
		t.Fatalf("GeoIP coordinates should remain: %#v", location)
	}
}

func TestGeoIPPrimaryKeepsGeoIPSource(t *testing.T) {
	config.Load("../settings.toml")

	oldIPLookup := ipInfoLookup
	oldGeoLookup := geoIPLookup
	ipInfoLookup = func(string) results.IPInfoResponse {
		return results.IPInfoResponse{
			IP:      "128.94.2.134",
			Country: "US",
		}
	}
	geoIPLookup = func(string) *GeoIPData {
		latitude := 36.6676
		longitude := -78.3875
		return &GeoIPData{
			CountryName: "United States",
			City:        "Boydton",
			Region:      "Virginia",
			Country:     "US",
			Postal:      "23917",
			Timezone:    "America/New_York",
			Latitude:    &latitude,
			Longitude:   &longitude,
			Location:    "36.668,-78.388",
		}
	}
	t.Cleanup(func() {
		ipInfoLookup = oldIPLookup
		geoIPLookup = oldGeoLookup
		config.Load("../settings.toml")
	})

	info, location := getIPInfoAndLocationByPriority("128.94.2.134")
	if info.Location != "36.668,-78.388" {
		t.Fatalf("valid GeoIP legacy loc should fill missing API loc: %q", info.Location)
	}
	if location == nil || location.Source != "geoip" {
		t.Fatalf("complete MMDB metadata should keep GeoIP source: %#v", location)
	}
	if location.City != "Boydton" || location.Region != "Virginia" {
		t.Fatalf("GeoIP fields should enrich API metadata: %#v", location)
	}
}

func TestInvalidAPILocIsReplacedByValidGeoIPLoc(t *testing.T) {
	config.Load("../settings.toml")

	oldIPLookup := ipInfoLookup
	oldGeoLookup := geoIPLookup
	ipInfoLookup = func(string) results.IPInfoResponse {
		return results.IPInfoResponse{
			IP:       "128.94.2.134",
			Country:  "US",
			Location: "91,-46.6339",
		}
	}
	geoIPLookup = func(string) *GeoIPData {
		latitude := 36.6676
		longitude := -78.3875
		return &GeoIPData{
			Country:   "US",
			City:      "Boydton",
			Latitude:  &latitude,
			Longitude: &longitude,
			Location:  "36.668,-78.388",
		}
	}
	t.Cleanup(func() {
		ipInfoLookup = oldIPLookup
		geoIPLookup = oldGeoLookup
		config.Load("../settings.toml")
	})

	info, location := getIPInfoAndLocationByPriority("128.94.2.134")
	if info.Location != "36.668,-78.388" {
		t.Fatalf("invalid API loc should be replaced by valid GeoIP loc: %q", info.Location)
	}
	if location == nil || location.Latitude == nil || location.Longitude == nil {
		t.Fatalf("valid GeoIP coordinates should survive invalid API loc: %#v", location)
	}
}

func TestCalculateDistanceWithUnknownServerLocation(t *testing.T) {
	oldCoord := serverCoord
	oldKnown := serverLocationKnown
	serverCoord.Lat = 0
	serverCoord.Lon = 0
	serverLocationKnown = false
	t.Cleanup(func() {
		serverCoord = oldCoord
		serverLocationKnown = oldKnown
	})

	if distance := calculateDistance("36.6676,-78.3875", "km"); distance != "" {
		t.Fatalf("unknown server location should suppress distance: %q", distance)
	}
}

func TestCountryNameOnlyAPIRecordIsReconciled(t *testing.T) {
	config.Load("../settings.toml")

	oldIPLookup := ipInfoLookup
	oldGeoLookup := geoIPLookup
	ipInfoLookup = func(string) results.IPInfoResponse {
		return results.IPInfoResponse{
			IP:          "128.94.2.134",
			CountryName: "Canada",
		}
	}
	geoIPLookup = func(string) *GeoIPData {
		latitude := 43.6532
		longitude := -79.3832
		return &GeoIPData{
			CountryName: "United States",
			Country:     "US",
			City:        "Toronto",
			Region:      "Ontario",
			Latitude:    &latitude,
			Longitude:   &longitude,
			Location:    "43.653,-79.383",
		}
	}
	t.Cleanup(func() {
		ipInfoLookup = oldIPLookup
		geoIPLookup = oldGeoLookup
		config.Load("../settings.toml")
	})

	info, location := getIPInfoAndLocationByPriority("128.94.2.134")
	if info.Country != "US" || info.CountryName != "United States" {
		t.Fatalf("final country code and name should be reconciled: %#v", info)
	}
	if location == nil || location.Country != "US" || location.CountryName != "United States" {
		t.Fatalf("location country fields should be reconciled: %#v", location)
	}
}

func TestMetroMismatchKeepsGeoIPPrimary(t *testing.T) {
	config.Load("../settings.toml")

	oldIPLookup := ipInfoLookup
	oldGeoLookup := geoIPLookup
	ipInfoLookup = func(string) results.IPInfoResponse {
		return results.IPInfoResponse{
			IP:       "128.94.2.134",
			City:     "Washington",
			Region:   "District of Columbia",
			Country:  "US",
			Location: "38.9072,-77.0369",
		}
	}
	geoIPLookup = func(string) *GeoIPData {
		latitude := 43.6532
		longitude := -79.3832
		return &GeoIPData{
			CountryName: "United States",
			City:        "Toronto",
			Region:      "Ontario",
			Country:     "US",
			Latitude:    &latitude,
			Longitude:   &longitude,
			Location:    "43.653,-79.383",
		}
	}
	t.Cleanup(func() {
		ipInfoLookup = oldIPLookup
		geoIPLookup = oldGeoLookup
		config.Load("../settings.toml")
	})

	info, location := getIPInfoAndLocationByPriority("128.94.2.134")
	if info.Location != "43.653,-79.383" {
		t.Fatalf("GeoIP loc should remain primary for a metro conflict: %q", info.Location)
	}
	if location == nil || location.City != "Toronto" {
		t.Fatalf("GeoIP metro should remain authoritative: %#v", location)
	}
	if location.Latitude == nil || *location.Latitude != 43.6532 ||
		location.Longitude == nil || *location.Longitude != -79.3832 {
		t.Fatalf("GeoIP metro coordinates should remain: %#v", location)
	}
}

func TestSetServerLocationAcceptsExplicitZeroCoordinates(t *testing.T) {
	tempFile := t.TempDir() + "/settings.toml"
	if err := os.WriteFile(tempFile, []byte("server_lat=0\nserver_lng=0\n"), 0o600); err != nil {
		t.Fatalf("write zero-coordinate config: %v", err)
	}
	conf := config.Load(tempFile)

	oldCoord := serverCoord
	oldKnown := serverLocationKnown
	t.Cleanup(func() {
		serverCoord = oldCoord
		serverLocationKnown = oldKnown
		config.Load("../settings.toml")
	})

	SetServerLocation(&conf)
	if !serverLocationKnown {
		t.Fatal("explicit (0,0) server coordinates should be marked known")
	}
	if serverCoord.Lat != 0 || serverCoord.Lon != 0 {
		t.Fatalf("unexpected explicit zero server coordinates: %#v", serverCoord)
	}
}

func TestServerLocationConfigurationRequiresBothKeys(t *testing.T) {
	tempFile := t.TempDir() + "/settings.toml"
	if err := os.WriteFile(tempFile, []byte("server_lat=12.34\n"), 0o600); err != nil {
		t.Fatalf("write partial server coordinate config: %v", err)
	}
	conf := config.Load(tempFile)
	if conf.ServerLocationConfigured {
		t.Fatal("one configured server coordinate key should not mark the pair explicit")
	}
}

func TestServerLocationConfigurationAcceptsEnvironmentPair(t *testing.T) {
	t.Setenv("SPEEDTEST_SERVER_LAT", "12.34")
	t.Setenv("SPEEDTEST_SERVER_LNG", "-56.78")
	tempFile := t.TempDir() + "/settings.toml"
	if err := os.WriteFile(tempFile, []byte("disable_ipinfo=false\n"), 0o600); err != nil {
		t.Fatalf("write environment server config: %v", err)
	}
	conf := config.Load(tempFile)
	if !conf.ServerLocationConfigured {
		t.Fatal("both environment server coordinates should mark the pair explicit")
	}
	if conf.ServerLat != 12.34 || conf.ServerLng != -56.78 {
		t.Fatalf("unexpected environment server coordinates: %.6f, %.6f", conf.ServerLat, conf.ServerLng)
	}
}

func TestMergeLocationsPreservesPrimaryCoordinates(t *testing.T) {
	primaryLat := 43.6532
	primaryLon := -79.3832
	fallbackLat := 38.9072
	fallbackLon := -77.0369
	primary := &results.Location{
		Country:     "US",
		CountryName: "United States",
		Latitude:    &primaryLat,
		Longitude:   &primaryLon,
		Source:      "geoip",
	}
	fallback := &results.Location{
		City:        "Washington",
		Region:      "District of Columbia",
		Country:     "US",
		CountryName: "United States",
		Latitude:    &fallbackLat,
		Longitude:   &fallbackLon,
		Source:      "ipinfo",
	}

	merged := mergeLocations(primary, fallback)
	if merged.City != "" || merged.Region != "" {
		t.Fatalf("fallback geography should not fill over existing primary coordinates: %#v", merged)
	}
	if merged.Latitude == nil || *merged.Latitude != primaryLat ||
		merged.Longitude == nil || *merged.Longitude != primaryLon {
		t.Fatalf("primary coordinates should remain authoritative: %#v", merged)
	}
}

func TestMergeLocationsAdoptsFallbackWithoutPrimaryGeography(t *testing.T) {
	fallbackLat := 38.9072
	fallbackLon := -77.0369
	primary := &results.Location{Country: "US", Source: "geoip"}
	fallback := &results.Location{
		City:        "Washington",
		Region:      "District of Columbia",
		Country:     "US",
		CountryName: "United States",
		Latitude:    &fallbackLat,
		Longitude:   &fallbackLon,
		Source:      "ipinfo",
	}
	merged := mergeLocations(primary, fallback)
	if merged.City != "Washington" || merged.Region != "District of Columbia" {
		t.Fatalf("fallback geography should fill missing primary city/region: %#v", merged)
	}
	if merged.Latitude == nil || *merged.Latitude != fallbackLat ||
		merged.Longitude == nil || *merged.Longitude != fallbackLon {
		t.Fatalf("fallback coordinates should fill missing primary coordinates: %#v", merged)
	}
}

func TestSyncLegacyLocationUsesFinalMergedPlace(t *testing.T) {
	lat := 38.9072
	lon := -77.0369
	info := results.IPInfoResponse{
		Country:  "US",
		City:     "Old City",
		Region:   "Old Region",
		Location: "43.6532,-79.3832",
	}
	location := &results.Location{
		City:        "Washington",
		Region:      "District of Columbia",
		Country:     "US",
		CountryName: "United States",
		Latitude:    &lat,
		Longitude:   &lon,
	}
	syncLegacyLocation(&info, location)
	if info.City != "Washington" || info.Region != "District of Columbia" ||
		info.Country != "US" || info.CountryName != "United States" ||
		info.Location != "38.907,-77.037" {
		t.Fatalf("legacy fields should sync to final location: %#v", info)
	}
}

func TestSyncLegacyLocationPreservesTextWhenCoordinatesMissing(t *testing.T) {
	info := results.IPInfoResponse{}
	location := &results.Location{
		City:        "Boydton",
		Region:      "Virginia",
		Country:     "US",
		CountryName: "United States",
	}
	syncLegacyLocation(&info, location)
	if info.City != "Boydton" || info.Region != "Virginia" ||
		info.Country != "US" || info.CountryName != "United States" ||
		info.Location != "" {
		t.Fatalf("legacy text should survive missing coordinates: %#v", info)
	}
}

func TestNormalizeIPInfoCountryNameReconcilesCountryCode(t *testing.T) {
	info := results.IPInfoResponse{
		Country:     "US",
		CountryName: "Canada",
	}
	normalizeIPInfoResponse(&info)
	if info.Country != "US" || info.CountryName != "United States" {
		t.Fatalf("country code/name should be reconciled: %#v", info)
	}
}

package web

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"
	log "github.com/sirupsen/logrus"
	"github.com/umahmood/haversine"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"github.com/librespeed/speedtest-go/config"
	"github.com/librespeed/speedtest-go/results"
)

var (
	serverCoord         haversine.Coord
	serverLocationKnown bool

	ipInfoHTTPClient = &http.Client{Timeout: 5 * time.Second}
	ipInfoBaseURL    = "https://ipinfo.io"
	ipInfoLookup     = getIPInfo
	geoIPLookup      = getGeoIPData
)

func getRandomData(length int) []byte {
	data := make([]byte, length)
	if _, err := rand.Read(data); err != nil {
		log.Fatalf("Failed to generate random data: %s", err)
	}
	return data
}

func getIPInfoURL(address string) string {
	apiKey := config.LoadedConfig().IPInfoAPIKey
	return getIPInfoURLWithKey(address, apiKey)
}

func getIPInfoURLWithKey(address, apiKey string) string {
	ipInfoURL := strings.TrimRight(ipInfoBaseURL, "/")
	if address != "" {
		ipInfoURL = fmt.Sprintf("%s/%s/json", ipInfoURL, url.PathEscape(address))
	} else {
		ipInfoURL += "/json"
	}

	if apiKey != "" {
		parsed, err := url.Parse(ipInfoURL)
		if err == nil {
			query := parsed.Query()
			query.Set("token", apiKey)
			parsed.RawQuery = query.Encode()
			ipInfoURL = parsed.String()
		}
	}

	return ipInfoURL
}

func getIPInfo(addr string) results.IPInfoResponse {
	var ret results.IPInfoResponse
	resp, err := ipInfoHTTPClient.Get(getIPInfoURL(addr))
	if err != nil {
		log.Errorf("Error getting response from ipinfo.io: %s", err)
		return ret
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Warnf("IPinfo returned HTTP status %d", resp.StatusCode)
		return ret
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		log.Errorf("Error reading response from ipinfo.io: %s", err)
		return ret
	}

	if err := json.Unmarshal(raw, &ret); err != nil {
		log.Errorf("Error parsing response from ipinfo.io: %s", err)
	}

	normalizeIPInfoResponse(&ret)
	return ret
}

func SetServerLocation(conf *config.Config) {
	serverLocationKnown = false
	serverCoord = haversine.Coord{}
	if conf.DisableIPInfo {
		log.Info("IP-derived location metadata is disabled")
		return
	}
	if conf.ServerLat != 0 || conf.ServerLng != 0 {
		if validCoordinates(conf.ServerLat, conf.ServerLng) {
			log.Infof("Configured server coordinates: %.6f, %.6f", conf.ServerLat, conf.ServerLng)
			serverCoord.Lat = conf.ServerLat
			serverCoord.Lon = conf.ServerLng
			serverLocationKnown = true
		} else {
			log.Errorf("Invalid configured server coordinates: latitude=%v longitude=%v", conf.ServerLat, conf.ServerLng)
		}
		return
	}

	ret := getIPInfo("")
	if ret.Location == "" {
		return
	}

	if ret.Location != "" {
		var err error
		serverCoord, err = parseLocationString(ret.Location)
		if err != nil {
			log.Errorf("Cannot get server coordinates: %s", err)
			return
		}
		serverLocationKnown = true
	}

	log.Infof("Fetched server coordinates: %.6f, %.6f", serverCoord.Lat, serverCoord.Lon)
}

func parseLocationString(location string) (haversine.Coord, error) {
	var coord haversine.Coord

	parts := strings.Split(strings.TrimSpace(location), ",")
	if len(parts) != 2 {
		err := fmt.Errorf("unknown location format: %s", location)
		log.Error(err)
		return coord, err
	}

	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		log.Errorf("Error parsing latitude: %s", parts[0])
		return coord, err
	}

	lng, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		log.Errorf("Error parsing longitude: %s", parts[1])
		return coord, err
	}

	if !validCoordinates(lat, lng) {
		err := fmt.Errorf("invalid coordinates: latitude=%v longitude=%v", lat, lng)
		log.Error(err)
		return coord, err
	}

	coord.Lat = lat
	coord.Lon = lng

	return coord, nil
}

func calculateDistance(clientLocation string, unit string) string {
	if !serverLocationKnown {
		return ""
	}
	clientCoord, err := parseLocationString(clientLocation)
	if err != nil {
		log.Errorf("Error parsing client coordinates: %s", err)
		return ""
	}

	dist, km := haversine.Distance(clientCoord, serverCoord)

	switch unit {
	case "km":
		dist = km
		rounded := roundToNearest10(dist)
		if dist < 20 {
			return "<20 km"
		}
		return fmt.Sprintf("%.0f km", rounded)
	case "NM":
		dist = km * 0.539957
		return fmt.Sprintf("%.2f NM", dist)
	default: // miles
		distMi := dist
		rounded := roundToNearest10(distMi)
		if distMi < 15 {
			return "<15 mi"
		}
		return fmt.Sprintf("%.0f mi", rounded)
	}
}

// roundToNearest10 rounds a float64 to the nearest 10, matching PHP round($d, -1)
func roundToNearest10(val float64) float64 {
	return float64(int64(val/10+0.5)) * 10
}

func validCoordinates(latitude, longitude float64) bool {
	return !math.IsNaN(latitude) &&
		!math.IsInf(latitude, 0) &&
		!math.IsNaN(longitude) &&
		!math.IsInf(longitude, 0) &&
		latitude >= -90 &&
		latitude <= 90 &&
		longitude >= -180 &&
		longitude <= 180
}

func parseValidatedCoordinates(location string) (float64, float64, bool) {
	parts := strings.Split(strings.TrimSpace(location), ",")
	if len(parts) != 2 {
		return 0, 0, false
	}

	latitude, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, false
	}
	longitude, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, false
	}
	if !validCoordinates(latitude, longitude) {
		return 0, 0, false
	}
	return latitude, longitude, true
}

func normalizeCountryCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != 2 {
		return ""
	}
	for _, char := range code {
		if char < 'A' || char > 'Z' {
			return ""
		}
	}
	return code
}

func countryNameFromCode(code string) string {
	code = normalizeCountryCode(code)
	if code == "" {
		return ""
	}
	region, err := language.ParseRegion(code)
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(display.English.Regions().Name(region))
	if name == "" || name == code {
		return ""
	}
	return name
}

func normalizeIPInfoResponse(info *results.IPInfoResponse) {
	info.IP = strings.TrimSpace(info.IP)
	info.Hostname = strings.TrimSpace(info.Hostname)
	info.City = strings.TrimSpace(info.City)
	info.Region = strings.TrimSpace(info.Region)
	info.Country = strings.TrimSpace(info.Country)
	info.CountryName = strings.TrimSpace(info.CountryName)
	info.Location = strings.TrimSpace(info.Location)
	info.Organization = strings.TrimSpace(info.Organization)
	info.ASName = strings.TrimSpace(info.ASName)
	info.Postal = strings.TrimSpace(info.Postal)
	info.Timezone = strings.TrimSpace(info.Timezone)
	info.Readme = strings.TrimSpace(info.Readme)

	if info.Organization == "" {
		info.Organization = info.ASName
	}

	countryCode := normalizeCountryCode(info.Country)
	if countryCode != "" {
		info.Country = countryCode
		if info.CountryName == "" {
			info.CountryName = countryNameFromCode(countryCode)
		}
	} else if info.Country != "" && info.CountryName == "" {
		info.CountryName = info.Country
		info.Country = ""
	}
}

// GeoIP database holder (lazily opened on first use)
var (
	geoIPReader     *maxminddb.Reader
	geoIPReaderFile string
	geoIPInitMu     sync.Mutex
)

type GeoIPData struct {
	ASName      string
	CountryName string
	City        string
	Region      string
	Country     string
	Postal      string
	Timezone    string
	Latitude    *float64
	Longitude   *float64
	Location    string
}

func mapStringValue(value map[string]interface{}, key string) string {
	if value == nil {
		return ""
	}
	if text, ok := value[key].(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func mapValue(value interface{}) map[string]interface{} {
	if value == nil {
		return nil
	}
	if result, ok := value.(map[string]interface{}); ok {
		return result
	}
	return nil
}

func stringValue(value interface{}) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func floatValue(value interface{}) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int32:
		return float64(number), true
	case int64:
		return float64(number), true
	case uint:
		return float64(number), true
	case uint32:
		return float64(number), true
	case uint64:
		return float64(number), true
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(number), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func localizedName(value interface{}) string {
	if text := stringValue(value); text != "" {
		return text
	}
	names := mapValue(value)
	if names == nil {
		return ""
	}
	if nested := mapValue(names["names"]); nested != nil {
		names = nested
	}
	if name := mapStringValue(names, "en"); name != "" {
		return name
	}
	for _, key := range []string{"default", "name", "code"} {
		if name := mapStringValue(names, key); name != "" {
			return name
		}
	}
	return ""
}

func firstSubdivisionName(value interface{}) string {
	switch subdivisions := value.(type) {
	case []interface{}:
		for _, subdivision := range subdivisions {
			if name := localizedName(subdivision); name != "" {
				return name
			}
		}
	case []map[string]interface{}:
		for _, subdivision := range subdivisions {
			if name := localizedName(subdivision); name != "" {
				return name
			}
		}
	}
	return ""
}

// getGeoIPData looks up the given IP in a configured country/ASN MMDB or
// city MMDB and normalizes common ipinfo.io and MaxMind field layouts.
func getGeoIPData(ipStr string) *GeoIPData {
	conf := config.LoadedConfig()
	if conf.DisableIPInfo || conf.GeoIPDatabaseFile == "" {
		return nil
	}

	geoIPInitMu.Lock()
	if geoIPReader != nil && geoIPReaderFile != conf.GeoIPDatabaseFile {
		_ = geoIPReader.Close()
		geoIPReader = nil
		geoIPReaderFile = ""
	}
	if geoIPReader == nil {
		if _, err := os.Stat(conf.GeoIPDatabaseFile); err != nil {
			if os.IsNotExist(err) {
				log.Warnf("GeoIP database file not found: %s", conf.GeoIPDatabaseFile)
			} else {
				log.Warnf("Cannot stat GeoIP database file: %s", err)
			}
			geoIPInitMu.Unlock()
			return nil
		}
		reader, err := maxminddb.Open(conf.GeoIPDatabaseFile)
		if err != nil {
			log.Warnf("Failed to open GeoIP database: %s", err)
			geoIPInitMu.Unlock()
			return nil
		}
		geoIPReader = reader
		geoIPReaderFile = conf.GeoIPDatabaseFile
	}
	reader := geoIPReader
	geoIPInitMu.Unlock()

	if reader == nil {
		return nil
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil
	}

	// Try ipinfo.io offline database format first
	var ipinfoResult map[string]interface{}
	if err := reader.Lookup(ip, &ipinfoResult); err != nil {
		log.Warnf("GeoIP lookup failed: %s", err)
		return nil
	}

	if len(ipinfoResult) == 0 {
		return nil
	}

	return normalizeGeoIPResult(ipinfoResult)
}

func normalizeGeoIPResult(ipinfoResult map[string]interface{}) *GeoIPData {
	if len(ipinfoResult) == 0 {
		return nil
	}

	result := &GeoIPData{}

	// ipinfo.io offline database format.
	result.ASName = firstNonEmpty(
		mapStringValue(ipinfoResult, "as_name"),
		mapStringValue(ipinfoResult, "autonomous_system_organization"),
		mapStringValue(ipinfoResult, "as_organization"),
		mapStringValue(ipinfoResult, "organization"),
		mapStringValue(ipinfoResult, "org"),
	)
	result.CountryName = firstNonEmpty(
		mapStringValue(ipinfoResult, "country_name"),
		mapStringValue(ipinfoResult, "countryName"),
	)
	result.Country = normalizeCountryCode(firstNonEmpty(
		mapStringValue(ipinfoResult, "country_code"),
		mapStringValue(ipinfoResult, "countryCode"),
		mapStringValue(ipinfoResult, "country"),
	))
	result.City = firstNonEmpty(
		mapStringValue(ipinfoResult, "city_name"),
		mapStringValue(ipinfoResult, "city"),
		localizedName(ipinfoResult["city"]),
	)
	result.Region = firstNonEmpty(
		mapStringValue(ipinfoResult, "region_name"),
		mapStringValue(ipinfoResult, "subdivision_name"),
		mapStringValue(ipinfoResult, "state_name"),
		mapStringValue(ipinfoResult, "region"),
		localizedName(ipinfoResult["region"]),
	)
	result.Postal = firstNonEmpty(
		mapStringValue(ipinfoResult, "postal_code"),
		mapStringValue(ipinfoResult, "postal"),
	)
	result.Timezone = firstNonEmpty(
		mapStringValue(ipinfoResult, "timezone"),
		mapStringValue(ipinfoResult, "time_zone"),
	)

	// MaxMind GeoIP2 City/ASN format.
	if result.ASName == "" {
		if autonomousSystem := mapValue(ipinfoResult["autonomous_system"]); autonomousSystem != nil {
			result.ASName = firstNonEmpty(
				mapStringValue(autonomousSystem, "organization"),
				mapStringValue(autonomousSystem, "name"),
			)
		}
	}
	if result.CountryName == "" {
		if country := mapValue(ipinfoResult["country"]); country != nil {
			result.CountryName = localizedName(country)
		}
	}
	if result.Country == "" {
		if country := mapValue(ipinfoResult["country"]); country != nil {
			result.Country = normalizeCountryCode(firstNonEmpty(
				mapStringValue(country, "iso_code"),
				mapStringValue(country, "code"),
			))
		}
	}
	if result.Country == "" {
		if country := stringValue(ipinfoResult["country"]); country != "" {
			result.Country = normalizeCountryCode(country)
			if result.CountryName == "" && result.Country == "" {
				result.CountryName = country
			}
		}
	}
	if result.CountryName == "" && result.Country != "" {
		result.CountryName = countryNameFromCode(result.Country)
	}

	if result.City == "" {
		if city := mapValue(ipinfoResult["city"]); city != nil {
			result.City = localizedName(city)
		}
	}
	if result.Region == "" {
		if subdivisions, ok := ipinfoResult["subdivisions"].([]interface{}); ok {
			result.Region = firstSubdivisionName(subdivisions)
		} else if subdivisions, ok := ipinfoResult["subdivisions"].([]map[string]interface{}); ok {
			result.Region = firstSubdivisionName(subdivisions)
		}
	}
	if result.Postal == "" {
		if postal := mapValue(ipinfoResult["postal"]); postal != nil {
			result.Postal = firstNonEmpty(
				mapStringValue(postal, "code"),
				mapStringValue(postal, "name"),
			)
		}
	}
	if result.Timezone == "" {
		if location := mapValue(ipinfoResult["location"]); location != nil {
			result.Timezone = firstNonEmpty(
				mapStringValue(location, "time_zone"),
				mapStringValue(location, "timezone"),
			)
		}
	}

	var latitude, longitude float64
	var latitudeOK, longitudeOK bool
	if location := mapValue(ipinfoResult["location"]); location != nil {
		latitude, latitudeOK = floatValue(location["latitude"])
		longitude, longitudeOK = floatValue(location["longitude"])
	}
	if !latitudeOK {
		latitude, latitudeOK = floatValue(ipinfoResult["latitude"])
	}
	if !longitudeOK {
		longitude, longitudeOK = floatValue(ipinfoResult["longitude"])
	}
	if !latitudeOK {
		latitude, latitudeOK = floatValue(ipinfoResult["lat"])
	}
	if !longitudeOK {
		longitude, longitudeOK = floatValue(ipinfoResult["lon"])
	}

	if !latitudeOK || !longitudeOK {
		if loc := firstNonEmpty(
			mapStringValue(ipinfoResult, "loc"),
			stringValue(ipinfoResult["location"]),
		); loc != "" {
			if parsedLat, parsedLon, valid := parseValidatedCoordinates(loc); valid {
				latitude = parsedLat
				longitude = parsedLon
				latitudeOK = true
				longitudeOK = true
			}
		}
	}

	if latitudeOK && longitudeOK && validCoordinates(latitude, longitude) {
		result.Latitude = float64Pointer(latitude)
		result.Longitude = float64Pointer(longitude)
		result.Location = fmt.Sprintf("%.6f,%.6f", latitude, longitude)
	}

	if result.ASName == "" &&
		result.CountryName == "" &&
		result.City == "" &&
		result.Region == "" &&
		result.Country == "" &&
		result.Postal == "" &&
		result.Timezone == "" &&
		result.Latitude == nil &&
		result.Longitude == nil {
		return nil
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func float64Pointer(value float64) *float64 {
	return &value
}

// getISPInfoByPriority tries to fetch ISP info using the ipinfo.io API first,
// then falls back to the configured offline GeoIP database, mirroring PHP behavior.
func getISPInfoByPriority(addr string) results.IPInfoResponse {
	info, _ := getIPInfoAndLocationByPriority(addr)
	return info
}

func locationFromIPInfo(info results.IPInfoResponse) *results.Location {
	normalizeIPInfoResponse(&info)
	location := &results.Location{
		City:        info.City,
		Region:      info.Region,
		Country:     normalizeCountryCode(info.Country),
		CountryName: info.CountryName,
		Postal:      info.Postal,
		Timezone:    info.Timezone,
		Source:      "ipinfo",
	}
	if latitude, longitude, valid := parseValidatedCoordinates(info.Location); valid {
		location.Latitude = float64Pointer(latitude)
		location.Longitude = float64Pointer(longitude)
	}
	if location.Country != "" && location.CountryName == "" {
		location.CountryName = countryNameFromCode(location.Country)
	}
	if location.Country == "" && location.CountryName != "" {
		// A non-ISO countryName is still useful, but country remains omitted.
	}
	if location.City == "" &&
		location.Region == "" &&
		location.Country == "" &&
		location.CountryName == "" &&
		location.Postal == "" &&
		location.Timezone == "" &&
		location.Latitude == nil &&
		location.Longitude == nil {
		return nil
	}
	return location
}

func locationFromGeoIPData(geo *GeoIPData) *results.Location {
	if geo == nil {
		return nil
	}
	location := &results.Location{
		City:        strings.TrimSpace(geo.City),
		Region:      strings.TrimSpace(geo.Region),
		Country:     normalizeCountryCode(geo.Country),
		CountryName: strings.TrimSpace(geo.CountryName),
		Postal:      strings.TrimSpace(geo.Postal),
		Timezone:    strings.TrimSpace(geo.Timezone),
		Source:      "geoip",
	}
	if geo.Latitude != nil && geo.Longitude != nil &&
		validCoordinates(*geo.Latitude, *geo.Longitude) {
		location.Latitude = float64Pointer(*geo.Latitude)
		location.Longitude = float64Pointer(*geo.Longitude)
	}
	if location.Country != "" && location.CountryName == "" {
		location.CountryName = countryNameFromCode(location.Country)
	}
	if location.City == "" &&
		location.Region == "" &&
		location.Country == "" &&
		location.CountryName == "" &&
		location.Postal == "" &&
		location.Timezone == "" &&
		location.Latitude == nil &&
		location.Longitude == nil {
		return nil
	}
	return location
}

func mergeLocations(primary, fallback *results.Location) *results.Location {
	if primary == nil {
		return fallback
	}
	if fallback == nil {
		return primary
	}
	contributed := false
	if primary.City == "" {
		if fallback.City != "" {
			primary.City = fallback.City
			contributed = true
		}
	}
	if primary.Region == "" {
		if fallback.Region != "" {
			primary.Region = fallback.Region
			contributed = true
		}
	}
	if primary.Country == "" {
		if fallback.Country != "" {
			primary.Country = fallback.Country
			contributed = true
		}
	}
	if primary.CountryName == "" {
		if fallback.CountryName != "" {
			primary.CountryName = fallback.CountryName
			contributed = true
		}
	}
	if primary.Postal == "" {
		if fallback.Postal != "" {
			primary.Postal = fallback.Postal
			contributed = true
		}
	}
	if primary.Timezone == "" {
		if fallback.Timezone != "" {
			primary.Timezone = fallback.Timezone
			contributed = true
		}
	}
	if primary.Latitude == nil {
		if fallback.Latitude != nil {
			primary.Latitude = fallback.Latitude
			contributed = true
		}
	}
	if primary.Longitude == nil {
		if fallback.Longitude != nil {
			primary.Longitude = fallback.Longitude
			contributed = true
		}
	}
	if contributed {
		primary.Source = "ipinfo+geoip"
	}
	return primary
}

func mergeGeoIPData(info *results.IPInfoResponse, geo *GeoIPData) {
	if geo == nil {
		return
	}
	if info.Organization == "" {
		info.Organization = geo.ASName
	}
	if info.ASName == "" {
		info.ASName = geo.ASName
	}
	if info.Location != "" {
		if _, _, valid := parseValidatedCoordinates(info.Location); !valid {
			info.Location = ""
		}
	}

	apiCountry := normalizeCountryCode(info.Country)
	geoCountry := normalizeCountryCode(geo.Country)
	countryConflict := apiCountry != "" && geoCountry != "" && apiCountry != geoCountry
	if countryConflict {
		normalizeIPInfoResponse(info)
		return
	}
	if info.Country == "" {
		info.Country = geo.Country
	}
	if info.CountryName == "" {
		info.CountryName = geo.CountryName
	}
	if info.City == "" {
		info.City = geo.City
	}
	if info.Region == "" {
		info.Region = geo.Region
	}
	if info.Postal == "" {
		info.Postal = geo.Postal
	}
	if info.Timezone == "" {
		info.Timezone = geo.Timezone
	}
	if info.Location == "" {
		info.Location = geo.Location
	}
	normalizeIPInfoResponse(info)
}

func countryCodesConflict(info results.IPInfoResponse, geo *GeoIPData) bool {
	if geo == nil {
		return false
	}
	apiCountry := normalizeCountryCode(info.Country)
	geoCountry := normalizeCountryCode(geo.Country)
	return apiCountry != "" && geoCountry != "" && apiCountry != geoCountry
}

func getIPInfoAndLocationByPriority(addr string) (results.IPInfoResponse, *results.Location) {
	conf := config.LoadedConfig()
	if conf.DisableIPInfo {
		info := results.IPInfoResponse{IP: strings.TrimSpace(addr)}
		return info, nil
	}

	info := ipInfoLookup(addr)
	normalizeIPInfoResponse(&info)
	apiLocation := locationFromIPInfo(info)

	geo := geoIPLookup(addr)
	geoLocation := locationFromGeoIPData(geo)
	if countryCodesConflict(info, geo) {
		geoLocation = nil
	}
	mergeGeoIPData(&info, geo)

	if info.IP == "" {
		info.IP = strings.TrimSpace(addr)
	}
	location := mergeLocations(apiLocation, geoLocation)
	return info, location
}

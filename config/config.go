package config

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type Config struct {
	BindAddress       string  `mapstructure:"bind_address"`
	Port              string  `mapstructure:"listen_port"`
	BaseURL           string  `mapstructure:"url_base"`
	ProxyProtocolPort string  `mapstructure:"proxyprotocol_port"`
	ServerLat         float64 `mapstructure:"server_lat"`
	ServerLng         float64 `mapstructure:"server_lng"`
	IPInfoAPIKey      string  `mapstructure:"ipinfo_api_key"`

	StatsPassword string `mapstructure:"statistics_password"`
	RedactIP      bool   `mapstructure:"redact_ip_addresses"`

	AssetsPath string `mapstructure:"assets_path"`

	DatabaseType     string `mapstructure:"database_type"`
	DatabaseHostname string `mapstructure:"database_hostname"`
	DatabaseName     string `mapstructure:"database_name"`
	DatabaseUsername string `mapstructure:"database_username"`
	DatabasePassword string `mapstructure:"database_password"`

	DatabaseFile string `mapstructure:"database_file"`

	EnableHTTP2 bool   `mapstructure:"enable_http2"`
	EnableTLS   bool   `mapstructure:"enable_tls"`
	TLSCertFile string `mapstructure:"tls_cert_file"`
	TLSKeyFile  string `mapstructure:"tls_key_file"`
}

var (
	configFile   string
	loadedConfig *Config = nil
)

func init() {
	viper.SetDefault("listen_port", "8989")
	viper.SetDefault("url_base", "")
	viper.SetDefault("proxyprotocol_port", "0")
	viper.SetDefault("download_chunks", 4)
	viper.SetDefault("distance_unit", "K")
	viper.SetDefault("enable_cors", false)
	viper.SetDefault("statistics_password", "PASSWORD")
	viper.SetDefault("redact_ip_addresses", false)
	viper.SetDefault("database_type", "postgresql")
	viper.SetDefault("database_hostname", "localhost")
	viper.SetDefault("database_name", "speedtest")
	viper.SetDefault("database_username", "postgres")
	viper.SetDefault("enable_tls", false)
	viper.SetDefault("enable_http2", false)

	viper.SetConfigName("settings")
	viper.AddConfigPath(".")
}

func Load(configPath string) Config {
	var conf Config

	configFile = configPath
	viper.SetConfigFile(configPath)
	viper.SetEnvPrefix("speedtest")
	viper.AutomaticEnv()
	viper.ReadInConfig()

	if err := viper.Unmarshal(&conf); err != nil {
		log.Fatalf("Error parsing config: %s", err)
	}

	loadedConfig = &conf

	return conf
}

func LoadedConfig() *Config {
	if loadedConfig == nil {
		Load(configFile)
	}
	return loadedConfig
}

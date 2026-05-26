package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server ServerConfig `mapstructure:"server"`
	TLS    TLSConfig    `mapstructure:"tls"`
	DB     DBConfig     `mapstructure:"db"`
	Log    LogConfig    `mapstructure:"log"`
	UI     UIConfig     `mapstructure:"ui"`
}

type UIConfig struct {
	FooterText           string `mapstructure:"footer_text"`
	FooterURL            string `mapstructure:"footer_url"`
	FooterAttribution    string `mapstructure:"footer_attribution"`
	FooterAttributionURL string `mapstructure:"footer_attribution_url"`
}

type ServerConfig struct {
	HTTPPort     int  `mapstructure:"http_port"`
	HTTPSPort    int  `mapstructure:"https_port"`
	RedirectHTTP bool `mapstructure:"redirect_http"`
}

type TLSConfig struct {
	Mode         string `mapstructure:"mode"`
	CertFile     string `mapstructure:"cert_file"`
	KeyFile      string `mapstructure:"key_file"`
	ACMEDomain   string `mapstructure:"acme_domain"`
	ACMECacheDir string `mapstructure:"acme_cache_dir"`
}

type DBConfig struct {
	Path string `mapstructure:"path"`
}

type LogConfig struct {
	Level string `mapstructure:"level"`
}

func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	v.SetDefault("server.http_port", 80)
	v.SetDefault("server.https_port", 443)
	v.SetDefault("server.redirect_http", true)
	v.SetDefault("tls.mode", "disabled")
	v.SetDefault("tls.acme_cache_dir", "/etc/yaamon/acme")
	v.SetDefault("db.path", "/etc/yaamon/yaamon.db")
	v.SetDefault("log.level", "info")
	v.SetDefault("ui.footer_text", "Yet Another AllstarLink favorites and MONitor tool")
	v.SetDefault("ui.footer_url", "")
	v.SetDefault("ui.footer_attribution", "N2VLV")
	v.SetDefault("ui.footer_attribution_url", "https://n2vlv.net")

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.AddConfigPath("/etc/yaamon")
		v.AddConfigPath(".")
		v.SetConfigName("config")
		v.SetConfigType("yaml")
	}

	// YAAMON_SERVER_HTTP_PORT, etc.
	v.SetEnvPrefix("YAAMON")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return &cfg, validate(&cfg)
}

func validate(cfg *Config) error {
	switch cfg.TLS.Mode {
	case "disabled", "self_signed", "provided", "acme":
	default:
		return fmt.Errorf("tls.mode must be one of: disabled, self_signed, provided, acme (got %q)", cfg.TLS.Mode)
	}
	if cfg.TLS.Mode == "provided" {
		if cfg.TLS.CertFile == "" {
			return fmt.Errorf("tls.cert_file is required when tls.mode=provided")
		}
		if cfg.TLS.KeyFile == "" {
			return fmt.Errorf("tls.key_file is required when tls.mode=provided")
		}
	}
	if cfg.TLS.Mode == "acme" && cfg.TLS.ACMEDomain == "" {
		return fmt.Errorf("tls.acme_domain is required when tls.mode=acme")
	}
	if cfg.Server.HTTPPort < 1 || cfg.Server.HTTPPort > 65535 {
		return fmt.Errorf("server.http_port must be 1–65535")
	}
	if cfg.TLS.Mode != "disabled" && (cfg.Server.HTTPSPort < 1 || cfg.Server.HTTPSPort > 65535) {
		return fmt.Errorf("server.https_port must be 1–65535")
	}
	return nil
}

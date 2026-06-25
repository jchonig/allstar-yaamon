package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// DefaultFooterURL is set at build time via -ldflags "-X allstar-yaamon/internal/config.DefaultFooterURL=...".
// It is used as the default value for ui.footer_url when not specified in config.yaml.
var DefaultFooterURL string

type Config struct {
	Server        ServerConfig        `mapstructure:"server"`
	TLS           TLSConfig           `mapstructure:"tls"`
	DB            DBConfig            `mapstructure:"db"`
	AstDB         AstDBConfig         `mapstructure:"astdb"`
	Log           LogConfig           `mapstructure:"log"`
	UI            UIConfig            `mapstructure:"ui"`
	Commands      CommandsConfig      `mapstructure:"commands"`
	ProxyAuth     ProxyAuthConfig     `mapstructure:"proxy_auth"`
	TailscaleAuth TailscaleAuthConfig `mapstructure:"tailscale_auth"`
	WebAuthn      WebAuthnConfig      `mapstructure:"webauthn"`

	configFile string // resolved path of the config file actually loaded
}

// CommandArg describes a user-supplied argument for a node command.
type CommandArg struct {
	Name  string `mapstructure:"name"`  // template placeholder key
	Label string `mapstructure:"label"` // UI label
	Type  string `mapstructure:"type"`  // "node_number" | "string"
}

// NodeCommand is a single entry in the Functions menu.
type NodeCommand struct {
	Name  string       `mapstructure:"name"`
	Cmd   string       `mapstructure:"cmd"`   // template; {node} always server-resolved
	Args  []CommandArg `mapstructure:"args"`
	Role  string       `mapstructure:"role"`  // "readonly" | "readwrite" | "admin" | "superuser"
	Group string       `mapstructure:"group"` // optional grouping key; divider inserted when group changes
}

// CommandsConfig holds the list of node commands shown in the Functions menu.
type CommandsConfig struct {
	Commands []NodeCommand `mapstructure:"commands"`
}

type WebAuthnConfig struct {
	RPID      string   `mapstructure:"rpid"`
	RPOrigins []string `mapstructure:"rp_origins"`
}

type ProxyAuthConfig struct {
	Enabled          bool              `mapstructure:"enabled"`
	UsernameHeader   string            `mapstructure:"username_header"`
	GroupsHeader     string            `mapstructure:"groups_header"`
	GroupRoles map[string]string `mapstructure:"group_roles"`
	CreateUsers      bool              `mapstructure:"create_users"`
	UpdateDBRole     bool              `mapstructure:"update_db_role"`
}

type TailscaleAuthConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	UserHeader   string `mapstructure:"user_header"`
	NameHeader   string `mapstructure:"name_header"`
	AvatarHeader string `mapstructure:"avatar_header"`
}

// ConfigFile returns the path of the config file that was loaded, if any.
func (c *Config) ConfigFile() string { return c.configFile }

type UIConfig struct {
	FooterText           string `mapstructure:"footer_text"`
	FooterURL            string `mapstructure:"footer_url"`
	FooterAttribution    string `mapstructure:"footer_attribution"`
	FooterAttributionURL string `mapstructure:"footer_attribution_url"`
}

type ServerConfig struct {
	HTTPPort             int    `mapstructure:"http_port"`
	HTTPSPort            int    `mapstructure:"https_port"`
	RedirectHTTP         bool   `mapstructure:"redirect_http"`
	QUIC                 bool   `mapstructure:"quic"`
	BindAddress          string `mapstructure:"bind_address"`
	BasePath             string `mapstructure:"base_path"`
	AllowPublicPlaintext bool   `mapstructure:"allow_public_plaintext"`
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

type AstDBConfig struct {
	Path   string `mapstructure:"path"`
	Update bool   `mapstructure:"update"`
}

type LogConfig struct {
	Level string `mapstructure:"level"`
}

func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	v.SetDefault("server.http_port", 8080)
	v.SetDefault("server.https_port", 443)
	v.SetDefault("server.redirect_http", true)
	v.SetDefault("server.quic", true)
	v.SetDefault("server.bind_address", "")
	v.SetDefault("server.base_path", "")
	v.SetDefault("server.allow_public_plaintext", false)
	v.SetDefault("tls.mode", "disabled")
	v.SetDefault("tls.acme_cache_dir", "/etc/yaamon/acme")
	v.SetDefault("db.path", "/var/lib/yaamon/yaamon.db")
	v.SetDefault("astdb.path", "/var/lib/yaamon/astdb.txt")
	v.SetDefault("astdb.update", true)
	v.SetDefault("log.level", "info")
	v.SetDefault("ui.footer_text", "Yet Another Allstarlink MONitor (and favorites)")
	v.SetDefault("ui.footer_url", DefaultFooterURL)
	v.SetDefault("ui.footer_attribution", "N2VLV")
	v.SetDefault("ui.footer_attribution_url", "https://n2vlv.net")
	v.SetDefault("proxy_auth.enabled", false)
	v.SetDefault("proxy_auth.username_header", "X-Auth-Request-Preferred-Username")
	v.SetDefault("proxy_auth.groups_header", "X-Auth-Request-Groups")
	v.SetDefault("proxy_auth.create_users", true)
	v.SetDefault("proxy_auth.update_db_role", false)
	v.SetDefault("tailscale_auth.enabled", false)
	v.SetDefault("tailscale_auth.user_header", "Tailscale-User-Login")
	v.SetDefault("tailscale_auth.name_header", "Tailscale-User-Name")
	v.SetDefault("tailscale_auth.avatar_header", "Tailscale-User-Profile-Pic")

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

	cfg.configFile = v.ConfigFileUsed()
	if len(cfg.Commands.Commands) == 0 {
		cfg.Commands.Commands = defaultCommands()
	}
	if err := normalise(&cfg); err != nil {
		return nil, err
	}
	return &cfg, validate(&cfg)
}

func defaultCommands() []NodeCommand {
	return []NodeCommand{
		{Name: "Say Time of Day", Cmd: "rpt cmd {node} status 12 xxx", Role: "readwrite", Group: "announce"},
		{Name: "Force ID", Cmd: "rpt cmd {node} status 11 xxx", Role: "readwrite", Group: "announce"},
		{Name: "Reconnect", Cmd: "rpt cmd {node} ilink 16", Role: "readwrite", Group: "link"},
		{Name: "Show Node Status", Cmd: "rpt stats {node}", Role: "readwrite", Group: "status"},
		{Name: "Show Link Status", Cmd: "rpt lstats {node}", Role: "readwrite", Group: "status"},
		{Name: "Show IAX Registry", Cmd: "iax2 show registry", Role: "readwrite", Group: "status"},
		{Name: "Show IAX Channels", Cmd: "iax2 show channels", Role: "readwrite", Group: "status"},
		{Name: "Show Network Status", Cmd: "iax2 show netstats", Role: "readwrite", Group: "status"},
		{Name: "Show Uptime", Cmd: "core show uptime", Role: "readwrite", Group: "status"},
	}
}

func normalise(cfg *Config) error {
	bp := strings.TrimRight(cfg.Server.BasePath, "/")
	if bp != "" && !strings.HasPrefix(bp, "/") {
		bp = "/" + bp
	}
	cfg.Server.BasePath = bp
	return nil
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
	validRoles := map[string]bool{"readonly": true, "readwrite": true, "admin": true, "superuser": true}
	for i, cmd := range cfg.Commands.Commands {
		if !validRoles[cmd.Role] {
			return fmt.Errorf("commands.commands[%d].role must be one of: readonly, readwrite, admin, superuser (got %q)", i, cmd.Role)
		}
	}
	return nil
}

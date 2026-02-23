package config

import (
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	RPGMaker RPGMakerConfig `mapstructure:"rpgmaker"`
	Database DatabaseConfig `mapstructure:"database"`
	Cache    CacheConfig    `mapstructure:"cache"`
	Game     GameConfig     `mapstructure:"game"`
	Security SecurityConfig `mapstructure:"security"`
	Plugins  PluginsConfig  `mapstructure:"plugins"`
	Script   ScriptConfig   `mapstructure:"script"`
}

type ServerConfig struct {
	Port      int    `mapstructure:"port"`
	Debug     bool   `mapstructure:"debug"`
	DebugPort int    `mapstructure:"debug_port"`
	AdminKey  string `mapstructure:"admin_key"`
	GameDir   string `mapstructure:"game_dir"` // Path to RMMV www/ directory (served at /)
}

type RPGMakerConfig struct {
	DataPath string `mapstructure:"data_path"`
	ImgPath  string `mapstructure:"img_path"`
}

type DatabaseConfig struct {
	Mode         string        `mapstructure:"mode"` // embedded_xml | embedded_memory | sqlite | mysql
	EmbeddedPath string        `mapstructure:"embedded_path"`
	SQLitePath   string        `mapstructure:"sqlite_path"`
	MySQLDSN     string        `mapstructure:"mysql_dsn"`
	MySQLMaxOpen int           `mapstructure:"mysql_max_open"`
	MySQLMaxIdle int           `mapstructure:"mysql_max_idle"`
	MySQLMaxLife time.Duration `mapstructure:"mysql_max_life"`
}

type CacheConfig struct {
	RedisAddr       string        `mapstructure:"redis_addr"`
	RedisPassword   string        `mapstructure:"redis_password"`
	RedisDB         int           `mapstructure:"redis_db"`
	LocalGCInterval time.Duration `mapstructure:"local_gc_interval"`
	LocalPubSubBuf  int           `mapstructure:"local_pubsub_buf"`
}

type GameConfig struct {
	MapTickMs            int  `mapstructure:"map_tick_ms"`
	SaveIntervalS        int  `mapstructure:"save_interval_s"`
	MaxPartySize         int  `mapstructure:"max_party_size"`
	PVPEnabled           bool `mapstructure:"pvp_enabled"`
	StartMapID           int  `mapstructure:"start_map_id"`
	StartX               int  `mapstructure:"start_x"`
	StartY               int  `mapstructure:"start_y"`
	ProtectionMs         int  `mapstructure:"protection_ms"`
	DropLifetimeS        int  `mapstructure:"drop_lifetime_s"`
	ChatNearbyRange      int  `mapstructure:"chat_nearby_range"`
	GlobalChatCooldownS  int  `mapstructure:"global_chat_cooldown_s"`
}

type SecurityConfig struct {
	JWTSecret      string        `mapstructure:"jwt_secret"`
	JWTTTLH        time.Duration `mapstructure:"jwt_ttl_h"`
	RateLimitRPS   float64       `mapstructure:"rate_limit_rps"`
	RateLimitBurst int           `mapstructure:"rate_limit_burst"`
	// AllowedOrigins lists the WebSocket/SSE origins that are permitted.
	// An empty slice allows all origins (useful for local development only).
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

type PluginsConfig struct {
	Dir       string `mapstructure:"dir"`
	ClientDir string `mapstructure:"client_dir"` // Path to client JS plugins (served at /plugins/*)
}

type ScriptConfig struct {
	VMPoolSize int           `mapstructure:"vm_pool_size"`
	Timeout    time.Duration `mapstructure:"timeout"`
}

// Load reads config from the given YAML file path.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	// Defaults
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.debug", false)
	v.SetDefault("database.mode", "embedded_xml")
	v.SetDefault("database.embedded_path", "./data/db")
	v.SetDefault("database.sqlite_path", "./data/game.db")
	v.SetDefault("database.mysql_max_open", 50)
	v.SetDefault("database.mysql_max_idle", 10)
	v.SetDefault("database.mysql_max_life", "1h")
	v.SetDefault("cache.local_gc_interval", "30s")
	v.SetDefault("cache.local_pubsub_buf", 256)
	v.SetDefault("game.map_tick_ms", 50)
	v.SetDefault("game.save_interval_s", 300)
	v.SetDefault("game.max_party_size", 4)
	v.SetDefault("game.start_map_id", 1)
	v.SetDefault("game.start_x", 5)
	v.SetDefault("game.start_y", 5)
	v.SetDefault("game.protection_ms", 3000)
	v.SetDefault("game.drop_lifetime_s", 300)
	v.SetDefault("game.chat_nearby_range", 10)
	v.SetDefault("game.global_chat_cooldown_s", 180)
	v.SetDefault("security.jwt_ttl_h", "72h")
	v.SetDefault("security.rate_limit_rps", 100)
	v.SetDefault("security.rate_limit_burst", 200)
	v.SetDefault("plugins.client_dir", "../client_plugin")
	v.SetDefault("script.vm_pool_size", 8)
	v.SetDefault("script.timeout", "5s")

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

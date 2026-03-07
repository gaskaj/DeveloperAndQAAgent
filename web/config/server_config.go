package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// ServerConfig holds all configuration for the control plane server.
type ServerConfig struct {
	Server     HTTPConfig     `mapstructure:"server"`
	Database   DatabaseConfig `mapstructure:"database"`
	Auth       AuthConfig     `mapstructure:"auth"`
	CORS       CORSConfig     `mapstructure:"cors"`
	Logging    LogConfig      `mapstructure:"logging"`
	Versioning VersionConfig  `mapstructure:"versioning"`
}

// HTTPConfig holds HTTP server settings.
type HTTPConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// Addr returns the listen address.
func (c HTTPConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	Name            string        `mapstructure:"name"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	SSLMode         string        `mapstructure:"sslmode"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	Pool            PoolConfig    `mapstructure:"pool"`
	Performance     PerfConfig    `mapstructure:"performance"`
}

// PoolConfig holds connection pool specific settings.
type PoolConfig struct {
	MaxConnections      int           `mapstructure:"max_connections"`
	MinConnections      int           `mapstructure:"min_connections"`
	MaxIdleTime         time.Duration `mapstructure:"max_idle_time"`
	HealthCheckPeriod   time.Duration `mapstructure:"health_check_period"`
	MaxConnLifetime     time.Duration `mapstructure:"max_conn_lifetime"`
	MaxConnIdleTime     time.Duration `mapstructure:"max_conn_idle_time"`
}

// PerfConfig holds query performance monitoring settings.
type PerfConfig struct {
	SlowQueryThreshold time.Duration `mapstructure:"slow_query_threshold"`
	QueryTimeout       time.Duration `mapstructure:"query_timeout"`
	EnableQueryLog     bool          `mapstructure:"enable_query_log"`
	EnableMetrics      bool          `mapstructure:"enable_metrics"`
}

// DSN returns the PostgreSQL connection string.
func (c DatabaseConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Name, c.SSLMode)
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	JWTSecret     string              `mapstructure:"jwt_secret"`
	JWTExpiry     time.Duration       `mapstructure:"jwt_expiry"`
	RefreshExpiry time.Duration       `mapstructure:"refresh_expiry"`
	BcryptCost    int                 `mapstructure:"bcrypt_cost"`
	Google        OAuthProviderConfig `mapstructure:"google"`
	Azure         OAuthProviderConfig `mapstructure:"azure"`
}

// OAuthProviderConfig holds OAuth provider settings.
type OAuthProviderConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	RedirectURL  string `mapstructure:"redirect_url"`
	TenantID     string `mapstructure:"tenant_id"` // Azure only
}

// CORSConfig holds CORS settings.
type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// VersionConfig holds API versioning settings.
type VersionConfig struct {
	DefaultVersion     string       `mapstructure:"default_version"`
	DeprecationWarning bool         `mapstructure:"deprecation_warning"`
	SupportedVersions  []APIVersion `mapstructure:"supported_versions"`
}

// APIVersion represents a supported API version.
type APIVersion struct {
	Version      string     `mapstructure:"version"`
	IsDefault    bool       `mapstructure:"is_default"`
	IsDeprecated bool       `mapstructure:"is_deprecated"`
	DeprecatedAt *time.Time `mapstructure:"deprecated_at"`
	SunsetAt     *time.Time `mapstructure:"sunset_at"`
}

// Load reads the server configuration from a YAML file.
func Load(path string) (*ServerConfig, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config %q: %w", path, err)
	}

	// Expand environment variables
	for _, key := range v.AllKeys() {
		val := v.GetString(key)
		if strings.Contains(val, "${") {
			expanded := os.Expand(val, os.Getenv)
			v.Set(key, expanded)
		}
	}

	var cfg ServerConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	// Apply defaults
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.ReadTimeout == 0 {
		cfg.Server.ReadTimeout = 30 * time.Second
	}
	if cfg.Server.WriteTimeout == 0 {
		cfg.Server.WriteTimeout = 30 * time.Second
	}
	if cfg.Server.ShutdownTimeout == 0 {
		cfg.Server.ShutdownTimeout = 15 * time.Second
	}
	if cfg.Database.Host == "" {
		cfg.Database.Host = "localhost"
	}
	if cfg.Database.SSLMode == "" {
		cfg.Database.SSLMode = "disable"
	}
	if cfg.Database.MaxOpenConns == 0 {
		cfg.Database.MaxOpenConns = 25
	}
	if cfg.Database.MaxIdleConns == 0 {
		cfg.Database.MaxIdleConns = 5
	}
	if cfg.Database.ConnMaxLifetime == 0 {
		cfg.Database.ConnMaxLifetime = 5 * time.Minute
	}
	if cfg.Database.Pool.MaxConnections == 0 {
		cfg.Database.Pool.MaxConnections = 25
	}
	if cfg.Database.Pool.MinConnections == 0 {
		cfg.Database.Pool.MinConnections = 5
	}
	if cfg.Database.Pool.MaxIdleTime == 0 {
		cfg.Database.Pool.MaxIdleTime = 30 * time.Minute
	}
	if cfg.Database.Pool.HealthCheckPeriod == 0 {
		cfg.Database.Pool.HealthCheckPeriod = 1 * time.Minute
	}
	if cfg.Database.Pool.MaxConnLifetime == 0 {
		cfg.Database.Pool.MaxConnLifetime = 1 * time.Hour
	}
	if cfg.Database.Pool.MaxConnIdleTime == 0 {
		cfg.Database.Pool.MaxConnIdleTime = 15 * time.Minute
	}
	if cfg.Database.Performance.SlowQueryThreshold == 0 {
		cfg.Database.Performance.SlowQueryThreshold = 100 * time.Millisecond
	}
	if cfg.Database.Performance.QueryTimeout == 0 {
		cfg.Database.Performance.QueryTimeout = 30 * time.Second
	}
	if !cfg.Database.Performance.EnableQueryLog {
		cfg.Database.Performance.EnableQueryLog = true
	}
	if !cfg.Database.Performance.EnableMetrics {
		cfg.Database.Performance.EnableMetrics = true
	}
	if cfg.Auth.JWTExpiry == 0 {
		cfg.Auth.JWTExpiry = 15 * time.Minute
	}
	if cfg.Auth.RefreshExpiry == 0 {
		cfg.Auth.RefreshExpiry = 7 * 24 * time.Hour
	}
	if cfg.Auth.BcryptCost == 0 {
		cfg.Auth.BcryptCost = 12
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Versioning.DefaultVersion == "" {
		cfg.Versioning.DefaultVersion = "v1"
	}
	if len(cfg.Versioning.SupportedVersions) == 0 {
		cfg.Versioning.SupportedVersions = []APIVersion{
			{
				Version:   "v1",
				IsDefault: true,
			},
		}
	}

	return &cfg, nil
}

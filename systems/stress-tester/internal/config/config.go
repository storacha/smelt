package config

import (
	"strings"

	"github.com/spf13/viper"
)

// Config holds all configuration for the stress tester
type Config struct {
	Guppy     GuppyConfig     `mapstructure:"guppy"`
	Store     StoreConfig     `mapstructure:"store"`
	Telemetry TelemetryConfig `mapstructure:"telemetry"`
	Generator GeneratorConfig `mapstructure:"generator"`
	Runner    RunnerConfig    `mapstructure:"runner"`
}

// GuppyConfig holds guppy CLI configuration
type GuppyConfig struct {
	Email      string `mapstructure:"email"`
	ConfigPath string `mapstructure:"config_path"`
	BinaryPath string `mapstructure:"binary_path"`
}

// StoreConfig holds database configuration
type StoreConfig struct {
	Type string `mapstructure:"type"` // "sqlite" or "postgres"
	Path string `mapstructure:"path"` // SQLite path
	DSN  string `mapstructure:"dsn"`  // PostgreSQL DSN
}

// TelemetryConfig holds telemetry configuration
type TelemetryConfig struct {
	PrometheusPort int    `mapstructure:"prometheus_port"`
	OTLPEndpoint   string `mapstructure:"otlp_endpoint"`
	ServiceName    string `mapstructure:"service_name"`
}

// GeneratorConfig holds random data generator configuration
type GeneratorConfig struct {
	Seed        int64  `mapstructure:"seed"`          // 0 = use timestamp
	MinFileSize string `mapstructure:"min_file_size"` // e.g., "256KB"
	MaxFileSize string `mapstructure:"max_file_size"` // e.g., "32MB"
}

// RunnerConfig holds runner configuration
type RunnerConfig struct {
	Upload   UploadRunnerConfig   `mapstructure:"upload"`
	Retrieve RetrieveRunnerConfig `mapstructure:"retrieve"`
}

// UploadRunnerConfig holds upload runner configurations
type UploadRunnerConfig struct {
	Burst      UploadBurstConfig      `mapstructure:"burst"`
	Continuous ContinuousUploadConfig `mapstructure:"continuous"`
}

// UploadBurstConfig holds burst mode upload configuration
type UploadBurstConfig struct {
	Spaces            int    `mapstructure:"spaces"`
	UploadsPerSpace   int    `mapstructure:"uploads_per_space"`
	ConcurrentUploads int    `mapstructure:"concurrent_uploads"`
	TotalSize         string `mapstructure:"total_size"` // e.g., "100MB"
}

// ContinuousUploadConfig holds continuous mode upload configuration
type ContinuousUploadConfig struct {
	TotalSize string `mapstructure:"total_size"` // Size per upload (e.g., "10MB")
	Interval  string `mapstructure:"interval"`   // Time between uploads (e.g., "1s")
	Duration  string `mapstructure:"duration"`   // Max runtime (e.g., "1h", "0" = forever)
	SpaceDID  string `mapstructure:"space_did"`  // Pin to specific space (optional)
}

// RetrieveRunnerConfig holds retrieve runner configurations
type RetrieveRunnerConfig struct {
	Burst      RetrieveBurstConfig      `mapstructure:"burst"`
	Continuous ContinuousRetrieveConfig `mapstructure:"continuous"`
}

// RetrieveBurstConfig holds burst mode retrieval configuration
type RetrieveBurstConfig struct {
	ConcurrentRetrievals int    `mapstructure:"concurrent_retrievals"`
	Limit                int    `mapstructure:"limit"`
	SpaceDID             string `mapstructure:"space_did"`
}

// ContinuousRetrieveConfig holds continuous mode retrieval configuration
type ContinuousRetrieveConfig struct {
	Interval string `mapstructure:"interval"`  // Time between retrievals (e.g., "1s")
	Duration string `mapstructure:"duration"`  // Max runtime (e.g., "1h", "0" = forever)
	SpaceDID string `mapstructure:"space_did"` // Filter by space (optional)
}

// Load loads configuration from the global viper instance.
// Config file and CLI flags should already be bound to global viper before calling this.
func Load() (*Config, error) {
	// Set defaults (lowest precedence in viper)
	setDefaults(viper.GetViper())

	// Environment variable binding (STRESS_ prefix)
	viper.SetEnvPrefix("STRESS")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// Guppy defaults
	v.SetDefault("guppy.email", "stress-test@test.local")
	v.SetDefault("guppy.binary_path", "guppy")
	v.SetDefault("guppy.config_path", "")

	// Store defaults
	v.SetDefault("store.type", "sqlite")
	v.SetDefault("store.path", "/data/stress.db")
	v.SetDefault("store.dsn", "")

	// Telemetry defaults
	v.SetDefault("telemetry.prometheus_port", 9090)
	v.SetDefault("telemetry.service_name", "stress-tester")
	v.SetDefault("telemetry.otlp_endpoint", "")

	// Generator defaults
	v.SetDefault("generator.seed", 0)
	v.SetDefault("generator.min_file_size", "256KB")
	v.SetDefault("generator.max_file_size", "32MB")

	// Upload burst mode defaults
	v.SetDefault("runner.upload.burst.spaces", 5)
	v.SetDefault("runner.upload.burst.uploads_per_space", 10)
	v.SetDefault("runner.upload.burst.concurrent_uploads", 5)
	v.SetDefault("runner.upload.burst.total_size", "10MB")

	// Retrieve burst mode defaults
	v.SetDefault("runner.retrieve.burst.concurrent_retrievals", 10)
	v.SetDefault("runner.retrieve.burst.limit", 0) // 0 = all
	v.SetDefault("runner.retrieve.burst.space_did", "")

	// Upload continuous mode defaults
	v.SetDefault("runner.upload.continuous.total_size", "10MB")
	v.SetDefault("runner.upload.continuous.interval", "1s")
	v.SetDefault("runner.upload.continuous.duration", "0") // 0 = forever
	v.SetDefault("runner.upload.continuous.space_did", "")

	// Retrieve continuous mode defaults
	v.SetDefault("runner.retrieve.continuous.interval", "1s")
	v.SetDefault("runner.retrieve.continuous.duration", "0") // 0 = forever
	v.SetDefault("runner.retrieve.continuous.space_did", "")
}

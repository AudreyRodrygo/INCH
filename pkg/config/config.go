// Package config provides a unified configuration loader for all Sentinel and Herald services.
//
// It follows the 12-Factor App methodology:
//   - Default values are set in code (reasonable for development)
//   - A YAML config file overrides defaults
//   - Environment variables override both (standard for Kubernetes/Docker)
//
// Environment variable mapping:
//
//	The prefix is configurable per service. For example, with prefix "COLLECTOR":
//	  COLLECTOR_PORT=8080        -> Config.Port = 8080
//	  COLLECTOR_LOG_LEVEL=debug  -> Config.LogLevel = "debug"
//
// Nested structs use underscores:
//
//	COLLECTOR_POSTGRES_HOST=db  -> Config.Postgres.Host = "db"
//
// Usage:
//
//	var cfg MyServiceConfig
//	if err := config.Load("COLLECTOR", "config.yaml", &cfg); err != nil {
//	    log.Fatal(err)
//	}
package config

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/spf13/viper"
)

// Load reads configuration from a YAML file and environment variables into dst.
//
// Parameters:
//   - envPrefix: prefix for environment variables (e.g., "COLLECTOR" -> COLLECTOR_PORT)
//   - configPath: path to the YAML config file (can be empty to skip file loading)
//   - dst: pointer to the config struct to populate
//
// Priority (highest wins): environment variables > config file > struct defaults.
//
// dst must be a pointer to a struct with `mapstructure` tags on its fields.
func Load(envPrefix, configPath string, dst any) error {
	v := viper.New()

	// Environment variables: COLLECTOR_PORT, COLLECTOR_LOG_LEVEL, etc.
	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Register all struct keys with Viper so AutomaticEnv can find them.
	// Without this, Viper only resolves env vars for keys it already knows
	// (from a config file or SetDefault). This ensures env-only mode works.
	bindStructKeys(v, dst, "")

	// Read YAML config file if path is provided.
	if configPath != "" {
		v.SetConfigFile(configPath)

		if err := v.ReadInConfig(); err != nil {
			// Config file is optional — missing file is not an error.
			// But a malformed file IS an error (we don't want silent misconfiguration).
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return fmt.Errorf("reading config file %s: %w", configPath, err)
			}
		}
	}

	// Unmarshal into the destination struct.
	if err := v.Unmarshal(dst); err != nil {
		return fmt.Errorf("unmarshaling config: %w", err)
	}

	return nil
}

// bindStructKeys walks the struct's fields and registers each one with Viper
// using SetDefault with a zero value. This makes AutomaticEnv aware of all
// possible keys, enabling env-only configuration without a YAML file.
//
// For nested structs, keys are joined with dots: "postgres.host".
// Viper maps these to env vars using the prefix and replacer: COLLECTOR_POSTGRES_HOST.
func bindStructKeys(v *viper.Viper, dst any, prefix string) {
	val := reflect.ValueOf(dst)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return
	}

	typ := val.Type()
	for i := range typ.NumField() {
		field := typ.Field(i)

		// Use the mapstructure tag if present, otherwise lowercase the field name.
		key := field.Tag.Get("mapstructure")
		if key == "" || key == "-" {
			key = strings.ToLower(field.Name)
		}

		if prefix != "" {
			key = prefix + "." + key
		}

		// Recurse into nested structs.
		if field.Type.Kind() == reflect.Struct {
			bindStructKeys(v, val.Field(i).Addr().Interface(), key)
			continue
		}

		// Register the key with a zero value so Viper knows about it.
		v.SetDefault(key, val.Field(i).Interface())
	}
}

// MustLoad is like Load but panics on error.
// Use only in main() where a missing config is unrecoverable.
func MustLoad(envPrefix, configPath string, dst any) {
	if err := Load(envPrefix, configPath, dst); err != nil {
		panic(fmt.Sprintf("config: %v", err))
	}
}

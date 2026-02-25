package config_test

import (
	"errors"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/config"
	"github.com/vinaysrao1/nest/internal/domain"
)

// TestLoadAllVarsSet verifies that all environment variables are parsed correctly
// when every variable is explicitly set.
func TestLoadAllVarsSet(t *testing.T) {
	setEnv(t, map[string]string{
		"DATABASE_URL":                  "postgres://localhost:5432/nest",
		"PORT":                          "9090",
		"SESSION_SECRET":                "supersecret",
		"WORKER_COUNT":                  "4",
		"RIVER_WORKER_COUNT":            "50",
		"RULE_TIMEOUT":                  "2s",
		"EVENT_TIMEOUT":                 "10s",
		"LOG_LEVEL":                     "debug",
		"DEV_MODE":                      "true",
		"COUNTER_BACKEND":               "postgres",
		"OPENAI_API_KEY":                "sk-test-key",
		"OPENAI_MODERATION_MODEL":       "text-moderation-latest",
		"OPENAI_MODERATION_TIMEOUT":     "10s",
		"OPENAI_MODERATION_MAX_INPUT":   "51200",
	})

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want 9090", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://localhost:5432/nest" {
		t.Error("DatabaseURL mismatch")
	}
	if cfg.SessionSecret != "supersecret" {
		t.Error("SessionSecret mismatch")
	}
	if cfg.WorkerCount != 4 {
		t.Errorf("WorkerCount = %d, want 4", cfg.WorkerCount)
	}
	if cfg.RiverWorkerCount != 50 {
		t.Errorf("RiverWorkerCount = %d, want 50", cfg.RiverWorkerCount)
	}
	if cfg.RuleTimeout != 2*time.Second {
		t.Errorf("RuleTimeout = %v, want 2s", cfg.RuleTimeout)
	}
	if cfg.EventTimeout != 10*time.Second {
		t.Errorf("EventTimeout = %v, want 10s", cfg.EventTimeout)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %s, want debug", cfg.LogLevel)
	}
	if !cfg.DevMode {
		t.Error("DevMode should be true")
	}
	if cfg.CounterBackend != "postgres" {
		t.Errorf("CounterBackend = %s, want postgres", cfg.CounterBackend)
	}
	if cfg.OpenAIAPIKey != "sk-test-key" {
		t.Errorf("OpenAIAPIKey = %s, want sk-test-key", cfg.OpenAIAPIKey)
	}
	if cfg.OpenAIModerationModel != "text-moderation-latest" {
		t.Errorf("OpenAIModerationModel = %s, want text-moderation-latest", cfg.OpenAIModerationModel)
	}
	if cfg.OpenAIModerationTimeout != 10*time.Second {
		t.Errorf("OpenAIModerationTimeout = %v, want 10s", cfg.OpenAIModerationTimeout)
	}
	if cfg.OpenAIModerationMaxInput != 51200 {
		t.Errorf("OpenAIModerationMaxInput = %d, want 51200", cfg.OpenAIModerationMaxInput)
	}
}

// TestLoadDefaults verifies that all optional fields take their documented
// default values when only the required variables are set.
func TestLoadDefaults(t *testing.T) {
	setEnv(t, map[string]string{
		"DATABASE_URL":   "postgres://localhost:5432/nest",
		"SESSION_SECRET": "secret",
	})

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("default Port = %d, want 8080", cfg.Port)
	}
	if cfg.WorkerCount != runtime.NumCPU() {
		t.Errorf("default WorkerCount = %d, want %d", cfg.WorkerCount, runtime.NumCPU())
	}
	if cfg.RiverWorkerCount != 100 {
		t.Errorf("default RiverWorkerCount = %d, want 100", cfg.RiverWorkerCount)
	}
	if cfg.RuleTimeout != time.Second {
		t.Errorf("default RuleTimeout = %v, want 1s", cfg.RuleTimeout)
	}
	if cfg.EventTimeout != 5*time.Second {
		t.Errorf("default EventTimeout = %v, want 5s", cfg.EventTimeout)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("default LogLevel = %s, want info", cfg.LogLevel)
	}
	if cfg.DevMode {
		t.Error("default DevMode should be false")
	}
	if cfg.CounterBackend != "memory" {
		t.Errorf("default CounterBackend = %s, want memory", cfg.CounterBackend)
	}
	if cfg.OpenAIAPIKey != "" {
		t.Errorf("default OpenAIAPIKey = %q, want empty string", cfg.OpenAIAPIKey)
	}
	if cfg.OpenAIModerationModel != "omni-moderation-latest" {
		t.Errorf("default OpenAIModerationModel = %s, want omni-moderation-latest", cfg.OpenAIModerationModel)
	}
	if cfg.OpenAIModerationTimeout != 5*time.Second {
		t.Errorf("default OpenAIModerationTimeout = %v, want 5s", cfg.OpenAIModerationTimeout)
	}
	if cfg.OpenAIModerationMaxInput != 102400 {
		t.Errorf("default OpenAIModerationMaxInput = %d, want 102400", cfg.OpenAIModerationMaxInput)
	}
}

// TestLoadMissingDatabaseURL verifies that Load returns a *domain.ConfigError
// when DATABASE_URL is not set.
func TestLoadMissingDatabaseURL(t *testing.T) {
	setEnv(t, map[string]string{})

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing DATABASE_URL")
	}
	var cfgErr *domain.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Errorf("expected *domain.ConfigError, got %T: %v", err, err)
	}
}

// TestLoadMissingSessionSecret verifies that Load returns a *domain.ConfigError
// when SESSION_SECRET is not set.
func TestLoadMissingSessionSecret(t *testing.T) {
	setEnv(t, map[string]string{
		"DATABASE_URL": "postgres://localhost:5432/nest",
	})

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing SESSION_SECRET")
	}
	var cfgErr *domain.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Errorf("expected *domain.ConfigError, got %T: %v", err, err)
	}
}

// TestLoadInvalidPort verifies that Load returns a *domain.ConfigError for
// various invalid PORT values: non-numeric, zero, out-of-range high, negative.
func TestLoadInvalidPort(t *testing.T) {
	tests := []struct {
		name    string
		portVal string
	}{
		{"non-numeric", "abc"},
		{"zero", "0"},
		{"too-high", "99999"},
		{"negative", "-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnv(t, map[string]string{
				"DATABASE_URL":   "postgres://localhost/nest",
				"SESSION_SECRET": "secret",
				"PORT":           tt.portVal,
			})

			_, err := config.Load()
			if err == nil {
				t.Fatalf("expected error for PORT=%q", tt.portVal)
			}
			var cfgErr *domain.ConfigError
			if !errors.As(err, &cfgErr) {
				t.Errorf("expected *domain.ConfigError, got %T: %v", err, err)
			}
		})
	}
}

// TestLoadInvalidOptionals verifies that Load returns a *domain.ConfigError
// for each invalid optional environment variable value.
func TestLoadInvalidOptionals(t *testing.T) {
	tests := []struct {
		name   string
		envKey string
		envVal string
	}{
		{"invalid WORKER_COUNT", "WORKER_COUNT", "abc"},
		{"invalid RIVER_WORKER_COUNT", "RIVER_WORKER_COUNT", "xyz"},
		{"invalid RULE_TIMEOUT", "RULE_TIMEOUT", "not-a-duration"},
		{"invalid EVENT_TIMEOUT", "EVENT_TIMEOUT", "bad"},
		{"invalid DEV_MODE", "DEV_MODE", "maybe"},
		{"invalid COUNTER_BACKEND", "COUNTER_BACKEND", "redis"},
		{"invalid OPENAI_MODERATION_TIMEOUT", "OPENAI_MODERATION_TIMEOUT", "not-a-duration"},
		{"invalid OPENAI_MODERATION_MAX_INPUT", "OPENAI_MODERATION_MAX_INPUT", "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnv(t, map[string]string{
				"DATABASE_URL":   "postgres://localhost/nest",
				"SESSION_SECRET": "secret",
				tt.envKey:        tt.envVal,
			})

			_, err := config.Load()
			if err == nil {
				t.Fatalf("expected error for invalid %s=%q", tt.envKey, tt.envVal)
			}
			var cfgErr *domain.ConfigError
			if !errors.As(err, &cfgErr) {
				t.Errorf("expected *domain.ConfigError, got %T: %v", err, err)
			}
		})
	}
}

// TestConfig_OpenAIModerationMaxInput_Zero verifies that setting
// OPENAI_MODERATION_MAX_INPUT to "0" returns a *domain.ConfigError because
// the value must be >= 1.
func TestConfig_OpenAIModerationMaxInput_Zero(t *testing.T) {
	setEnv(t, map[string]string{
		"DATABASE_URL":                "postgres://localhost/nest",
		"SESSION_SECRET":              "secret",
		"OPENAI_MODERATION_MAX_INPUT": "0",
	})

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for OPENAI_MODERATION_MAX_INPUT=0")
	}
	var cfgErr *domain.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Errorf("expected *domain.ConfigError, got %T: %v", err, err)
	}
}

// setEnv clears all config-related env vars, then sets the provided ones.
// Uses t.Cleanup to restore the original environment after the test.
func setEnv(t *testing.T, vars map[string]string) {
	t.Helper()

	keys := []string{
		"DATABASE_URL", "PORT", "SESSION_SECRET", "WORKER_COUNT",
		"RIVER_WORKER_COUNT", "RULE_TIMEOUT", "EVENT_TIMEOUT",
		"LOG_LEVEL", "DEV_MODE", "COUNTER_BACKEND",
		"OPENAI_API_KEY", "OPENAI_MODERATION_MODEL",
		"OPENAI_MODERATION_TIMEOUT", "OPENAI_MODERATION_MAX_INPUT",
	}

	// Save originals and clear all keys.
	originals := make(map[string]string)
	for _, k := range keys {
		originals[k] = os.Getenv(k)
		os.Unsetenv(k)
	}

	// Restore originals on cleanup.
	t.Cleanup(func() {
		for _, k := range keys {
			if v := originals[k]; v != "" {
				os.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	})

	// Set the requested variables.
	for k, v := range vars {
		os.Setenv(k, v)
	}
}

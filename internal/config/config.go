package config

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
)

// Config holds all runtime configuration parsed from environment variables.
type Config struct {
	Port                     int           // PORT, default 8080
	DatabaseURL              string        // DATABASE_URL, REQUIRED
	SessionSecret            string        // SESSION_SECRET, REQUIRED
	WorkerCount              int           // WORKER_COUNT, default runtime.NumCPU()
	RiverWorkerCount         int           // RIVER_WORKER_COUNT, default 100
	RuleTimeout              time.Duration // RULE_TIMEOUT, default 1s
	EventTimeout             time.Duration // EVENT_TIMEOUT, default 5s
	LogLevel                 string        // LOG_LEVEL, default "info"
	DevMode                  bool          // DEV_MODE, default false
	CounterBackend           string        // COUNTER_BACKEND, default "memory"
	OpenAIAPIKey             string        // OPENAI_API_KEY, optional
	OpenAIModerationModel    string        // OPENAI_MODERATION_MODEL, default "omni-moderation-latest"
	OpenAIModerationTimeout  time.Duration // OPENAI_MODERATION_TIMEOUT, default 5s
	OpenAIModerationMaxInput int           // OPENAI_MODERATION_MAX_INPUT, default 102400 (100KB)
}

// Load parses environment variables into a Config struct.
// Returns a *domain.ConfigError if required variables are missing or values
// are invalid.
//
// Required env vars:
//   - DATABASE_URL: PostgreSQL connection string
//   - SESSION_SECRET: secret key for session signing
//
// Optional env vars with defaults:
//   - PORT (8080): HTTP listen port, must be in range 1–65535
//   - WORKER_COUNT (runtime.NumCPU()): Starlark worker pool size
//   - RIVER_WORKER_COUNT (100): river background job worker count
//   - RULE_TIMEOUT (1s): per-rule evaluation timeout duration
//   - EVENT_TIMEOUT (5s): per-event total evaluation timeout duration
//   - LOG_LEVEL ("info"): logging verbosity
//   - DEV_MODE (false): enable development mode
//   - COUNTER_BACKEND ("memory"): counter storage backend, "memory" or "postgres"
//   - OPENAI_API_KEY (""): OpenAI API key; empty disables OpenAI moderation
//   - OPENAI_MODERATION_MODEL ("omni-moderation-latest"): OpenAI moderation model name
//   - OPENAI_MODERATION_TIMEOUT (5s): HTTP timeout for OpenAI moderation requests
//   - OPENAI_MODERATION_MAX_INPUT (102400): maximum input size in bytes for moderation, must be >= 1
//
// Errors:
//   - *domain.ConfigError if DATABASE_URL is empty
//   - *domain.ConfigError if SESSION_SECRET is empty
//   - *domain.ConfigError if PORT is not a valid integer or out of range 1–65535
//   - *domain.ConfigError if WORKER_COUNT or RIVER_WORKER_COUNT are not valid integers
//   - *domain.ConfigError if RULE_TIMEOUT or EVENT_TIMEOUT are not valid durations
//   - *domain.ConfigError if DEV_MODE is not parseable as bool
//   - *domain.ConfigError if COUNTER_BACKEND is not "memory" or "postgres"
//   - *domain.ConfigError if OPENAI_MODERATION_TIMEOUT is not a valid duration
//   - *domain.ConfigError if OPENAI_MODERATION_MAX_INPUT is not a valid integer or < 1
func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, &domain.ConfigError{Message: "DATABASE_URL is required"}
	}

	sessionSecret := os.Getenv("SESSION_SECRET")
	if sessionSecret == "" {
		return nil, &domain.ConfigError{Message: "SESSION_SECRET is required"}
	}

	cfg := &Config{
		Port:                     8080,
		DatabaseURL:              dbURL,
		SessionSecret:            sessionSecret,
		WorkerCount:              runtime.NumCPU(),
		RiverWorkerCount:         100,
		RuleTimeout:              time.Second,
		EventTimeout:             5 * time.Second,
		LogLevel:                 "info",
		DevMode:                  false,
		CounterBackend:           "memory",
		OpenAIModerationModel:    "omni-moderation-latest",
		OpenAIModerationTimeout:  5 * time.Second,
		OpenAIModerationMaxInput: 102400,
	}

	if err := parsePort(cfg); err != nil {
		return nil, err
	}

	if err := parseWorkerCount(cfg); err != nil {
		return nil, err
	}

	if err := parseRiverWorkerCount(cfg); err != nil {
		return nil, err
	}

	if err := parseRuleTimeout(cfg); err != nil {
		return nil, err
	}

	if err := parseEventTimeout(cfg); err != nil {
		return nil, err
	}

	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	if err := parseDevMode(cfg); err != nil {
		return nil, err
	}

	if err := parseCounterBackend(cfg); err != nil {
		return nil, err
	}

	cfg.OpenAIAPIKey = os.Getenv("OPENAI_API_KEY")

	if v := os.Getenv("OPENAI_MODERATION_MODEL"); v != "" {
		cfg.OpenAIModerationModel = v
	}

	if err := parseOpenAIModerationTimeout(cfg); err != nil {
		return nil, err
	}

	if err := parseOpenAIModerationMaxInput(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// parsePort parses the PORT environment variable into cfg.Port.
// Port must be an integer in the range 1–65535.
func parsePort(cfg *Config) error {
	v := os.Getenv("PORT")
	if v == "" {
		return nil
	}

	port, err := strconv.Atoi(v)
	if err != nil {
		return &domain.ConfigError{Message: "PORT must be a valid integer"}
	}

	if port < 1 || port > 65535 {
		return &domain.ConfigError{Message: fmt.Sprintf("PORT must be in range 1-65535, got %d", port)}
	}

	cfg.Port = port
	return nil
}

// parseWorkerCount parses the WORKER_COUNT environment variable into cfg.WorkerCount.
func parseWorkerCount(cfg *Config) error {
	v := os.Getenv("WORKER_COUNT")
	if v == "" {
		return nil
	}

	wc, err := strconv.Atoi(v)
	if err != nil {
		return &domain.ConfigError{Message: "WORKER_COUNT must be a valid integer"}
	}

	if wc < 1 {
		return &domain.ConfigError{Message: fmt.Sprintf("WORKER_COUNT must be >= 1, got %d", wc)}
	}

	cfg.WorkerCount = wc
	return nil
}

// parseRiverWorkerCount parses the RIVER_WORKER_COUNT environment variable into cfg.RiverWorkerCount.
func parseRiverWorkerCount(cfg *Config) error {
	v := os.Getenv("RIVER_WORKER_COUNT")
	if v == "" {
		return nil
	}

	rwc, err := strconv.Atoi(v)
	if err != nil {
		return &domain.ConfigError{Message: "RIVER_WORKER_COUNT must be a valid integer"}
	}

	if rwc < 1 {
		return &domain.ConfigError{Message: fmt.Sprintf("RIVER_WORKER_COUNT must be >= 1, got %d", rwc)}
	}

	cfg.RiverWorkerCount = rwc
	return nil
}

// parseRuleTimeout parses the RULE_TIMEOUT environment variable into cfg.RuleTimeout.
func parseRuleTimeout(cfg *Config) error {
	v := os.Getenv("RULE_TIMEOUT")
	if v == "" {
		return nil
	}

	d, err := time.ParseDuration(v)
	if err != nil {
		return &domain.ConfigError{Message: "RULE_TIMEOUT must be a valid duration (e.g., 1s, 500ms)"}
	}

	cfg.RuleTimeout = d
	return nil
}

// parseEventTimeout parses the EVENT_TIMEOUT environment variable into cfg.EventTimeout.
func parseEventTimeout(cfg *Config) error {
	v := os.Getenv("EVENT_TIMEOUT")
	if v == "" {
		return nil
	}

	d, err := time.ParseDuration(v)
	if err != nil {
		return &domain.ConfigError{Message: "EVENT_TIMEOUT must be a valid duration (e.g., 5s, 10s)"}
	}

	cfg.EventTimeout = d
	return nil
}

// parseDevMode parses the DEV_MODE environment variable into cfg.DevMode.
func parseDevMode(cfg *Config) error {
	v := os.Getenv("DEV_MODE")
	if v == "" {
		return nil
	}

	b, err := strconv.ParseBool(v)
	if err != nil {
		return &domain.ConfigError{Message: "DEV_MODE must be true or false"}
	}

	cfg.DevMode = b
	return nil
}

// parseCounterBackend parses the COUNTER_BACKEND environment variable into cfg.CounterBackend.
// Valid values are "memory" and "postgres".
func parseCounterBackend(cfg *Config) error {
	v := os.Getenv("COUNTER_BACKEND")
	if v == "" {
		return nil
	}

	if v != "memory" && v != "postgres" {
		return &domain.ConfigError{Message: `COUNTER_BACKEND must be "memory" or "postgres"`}
	}

	cfg.CounterBackend = v
	return nil
}

// parseOpenAIModerationTimeout parses the OPENAI_MODERATION_TIMEOUT environment variable
// into cfg.OpenAIModerationTimeout.
func parseOpenAIModerationTimeout(cfg *Config) error {
	v := os.Getenv("OPENAI_MODERATION_TIMEOUT")
	if v == "" {
		return nil
	}

	d, err := time.ParseDuration(v)
	if err != nil {
		return &domain.ConfigError{Message: "OPENAI_MODERATION_TIMEOUT must be a valid duration (e.g., 5s, 500ms)"}
	}

	cfg.OpenAIModerationTimeout = d
	return nil
}

// parseOpenAIModerationMaxInput parses the OPENAI_MODERATION_MAX_INPUT environment variable
// into cfg.OpenAIModerationMaxInput. The value must be an integer >= 1.
func parseOpenAIModerationMaxInput(cfg *Config) error {
	v := os.Getenv("OPENAI_MODERATION_MAX_INPUT")
	if v == "" {
		return nil
	}

	n, err := strconv.Atoi(v)
	if err != nil {
		return &domain.ConfigError{Message: "OPENAI_MODERATION_MAX_INPUT must be a valid integer"}
	}

	if n < 1 {
		return &domain.ConfigError{Message: fmt.Sprintf("OPENAI_MODERATION_MAX_INPUT must be >= 1, got %d", n)}
	}

	cfg.OpenAIModerationMaxInput = n
	return nil
}

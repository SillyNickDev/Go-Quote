package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type AppConfig struct {
	Mode          string `json:"mode"`
	DBPath        string `json:"db_path"`
	TwitchUser    string `json:"twitch_user"`
	TwitchOAuth   string `json:"twitch_oauth"`
	TwitchChannel string `json:"twitch_channel"`
}

const configFileName = "go-quote.config.json"

// setup merges defaults, persisted config, environment overrides (via applyEnvDefaults), and CLI flags,
// then writes the resolved configuration back to disk so users only enter credentials once.
func setup(mode, dbPath, user, oauth, channel string) (AppConfig, error) {
	defaults := AppConfig{
		Mode:   "twitch",
		DBPath: "quotes.db",
	}

	applyEnvDefaults(&mode, &dbPath, &user, &oauth, &channel)

	fileCfg, err := readConfigFile(configFileName)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return AppConfig{}, fmt.Errorf("reading config file: %w", err)
	}

	flagCfg := AppConfig{
		Mode:          strings.TrimSpace(mode),
		DBPath:        strings.TrimSpace(dbPath),
		TwitchUser:    strings.TrimSpace(user),
		TwitchOAuth:   strings.TrimSpace(oauth),
		TwitchChannel: strings.TrimSpace(channel),
	}

	finalCfg := mergeConfigs(defaults, fileCfg, flagCfg)
	finalCfg.Mode = strings.ToLower(finalCfg.Mode)

	if err := saveConfigFile(configFileName, finalCfg); err != nil {
		return AppConfig{}, fmt.Errorf("saving config file: %w", err)
	}

	return finalCfg, nil
}

func mergeConfigs(configs ...AppConfig) AppConfig {
	var merged AppConfig
	for _, cfg := range configs {
		if cfg.Mode != "" {
			merged.Mode = cfg.Mode
		}
		if cfg.DBPath != "" {
			merged.DBPath = cfg.DBPath
		}
		if cfg.TwitchUser != "" {
			merged.TwitchUser = cfg.TwitchUser
		}
		if cfg.TwitchOAuth != "" {
			merged.TwitchOAuth = cfg.TwitchOAuth
		}
		if cfg.TwitchChannel != "" {
			merged.TwitchChannel = cfg.TwitchChannel
		}
	}
	return merged
}

func readConfigFile(path string) (AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AppConfig{}, err
	}
	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return AppConfig{}, err
	}
	return cfg, nil
}

func saveConfigFile(path string, cfg AppConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// applyEnvDefaults populates missing CLI flag values from environment variables so users
// can set credentials once instead of passing them on every run. It trims whitespace and
// normalizes the mode to lower-case, defaulting to "twitch" when unset.
func applyEnvDefaults(mode, dbPath, user, oauth, channel *string) {
	pick := func(values ...string) string {
		for _, v := range values {
			if trimmed := strings.TrimSpace(v); trimmed != "" {
				return trimmed
			}
		}
		return ""
	}

	if mode != nil {
		*mode = pick(*mode, os.Getenv("GOQUOTE_MODE"))
		if *mode == "" {
			*mode = "twitch"
		}
		*mode = strings.ToLower(*mode)
	}

	if dbPath != nil {
		*dbPath = pick(*dbPath, os.Getenv("GOQUOTE_DB"), os.Getenv("QUOTE_DB"))
	}

	if user != nil {
		*user = pick(*user, os.Getenv("GOQUOTE_USER"), os.Getenv("TWITCH_USER"))
	}

	if oauth != nil {
		*oauth = pick(*oauth, os.Getenv("GOQUOTE_OAUTH"), os.Getenv("TWITCH_OAUTH"), os.Getenv("TWITCH_TOKEN"), os.Getenv("OAUTH_TOKEN"))
	}

	if channel != nil {
		*channel = pick(*channel, os.Getenv("GOQUOTE_CHANNEL"), os.Getenv("TWITCH_CHANNEL"))
	}
}

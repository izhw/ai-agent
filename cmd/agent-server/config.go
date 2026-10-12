package main

import (
	"errors"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type storageConfig struct {
	Driver      string `yaml:"driver"`
	DSN         string `yaml:"dsn"`
	AutoMigrate bool   `yaml:"auto_migrate"`
}

type modelConfig struct {
	Provider      string        `yaml:"provider"`
	Model         string        `yaml:"model"`
	BaseURL       string        `yaml:"base_url"`
	APIVersion    string        `yaml:"api_version"`
	CredentialRef string        `yaml:"credential_ref"`
	Timeout       time.Duration `yaml:"timeout"`
	Temperature   *float32      `yaml:"temperature"`
	MaxTokens     *int          `yaml:"max_tokens"`
}

type deepPresetConfig struct {
	MaxIterations          int  `yaml:"max_iterations"`
	WithoutWriteTodos      bool `yaml:"without_write_todos"`
	WithoutGeneralSubAgent bool `yaml:"without_general_sub_agent"`
}

type config struct {
	Address     string                      `yaml:"address"`
	Storage     storageConfig               `yaml:"storage"`
	Models      map[string]modelConfig      `yaml:"models"`
	DeepPresets map[string]deepPresetConfig `yaml:"deep_presets"`
}

func loadConfig(path string) (config, error) {
	var value config
	data, err := os.ReadFile(path)
	if err != nil {
		return value, err
	}
	if err = yaml.Unmarshal(data, &value); err != nil {
		return value, err
	}
	if value.Address == "" {
		value.Address = ":8080"
	}
	if value.Storage.Driver == "" {
		value.Storage.Driver = "memory"
	}
	if len(value.Models) == 0 {
		return value, errors.New("at least one model must be configured")
	}
	return value, nil
}

package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const configFile = ".unrealircd_tui_config"

type Config struct {
	SourceDir string `json:"source_dir"`
	BuildDir  string `json:"build_dir"`
	Version   string `json:"version"`
}

type Downloads struct {
	Src    string `json:"src"`
	Winssl string `json:"winssl"`
}

type VersionInfo struct {
	Type      string    `json:"type"`
	Version   string    `json:"version"`
	Downloads Downloads `json:"downloads"`
}

type UpdateResponse map[string]map[string]VersionInfo

func loadConfig() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(home, configFile)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, nil // No config file
	}
	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var config Config
	err = json.NewDecoder(file).Decode(&config)
	return &config, err
}

func saveConfig(config *Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configPath := filepath.Join(home, configFile)
	file, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(config)
}

func compareVersions(v1, v2 string) int {
	// Strip suffixes like -rc1, -git, etc. before comparison
	v1 = stripVersionSuffix(v1)
	v2 = stripVersionSuffix(v2)

	// Simple version comparison: split by "." and compare as ints
	p1 := strings.Split(v1, ".")
	p2 := strings.Split(v2, ".")
	for i := 0; i < len(p1) && i < len(p2); i++ {
		n1, _ := strconv.Atoi(p1[i])
		n2, _ := strconv.Atoi(p2[i])
		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}
	if len(p1) < len(p2) {
		return -1
	}
	if len(p1) > len(p2) {
		return 1
	}
	return 0
}

// stripVersionSuffix removes -rc1, -git, or any other -suffix from version strings
func stripVersionSuffix(version string) string {
	// Find the first dash and remove everything after it
	if idx := strings.Index(version, "-"); idx != -1 {
		return version[:idx]
	}
	return version
}
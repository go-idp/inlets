package config

import (
	"os"
	"path/filepath"
	"strings"
)

// SearchPaths defines default config file locations (priority order).
var SearchPaths = []string{
	"{CWD}/.go-idp/inlets.yaml",
	"{CWD}/.inlets.yaml",
	"{HOME}/.go-idp/inlets/config.yaml",
	"{HOME}/.config/inlets.yaml",
	"{HOME}/.config/inlets.yml",
	"/etc/go-idp/inlets/config.yaml",
	"/etc/inlets/config.yaml",
}

// FindFile returns the first existing config path from SearchPaths.
func FindFile() string {
	wd, _ := os.Getwd()
	home, _ := os.UserHomeDir()

	for _, template := range SearchPaths {
		path := template
		if strings.Contains(path, "{CWD}") {
			if wd == "" {
				continue
			}
			path = strings.ReplaceAll(path, "{CWD}", wd)
		}
		if strings.Contains(path, "{HOME}") {
			if home == "" {
				continue
			}
			path = strings.ReplaceAll(path, "{HOME}", home)
		}
		path = filepath.Clean(filepath.FromSlash(path))
		if fileExists(path) {
			return path
		}
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Dir returns the directory containing the config file.
func Dir(configPath string) string {
	if configPath == "" {
		wd, _ := os.Getwd()
		return wd
	}
	return filepath.Dir(configPath)
}

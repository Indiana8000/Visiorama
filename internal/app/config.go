package app

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Library    LibraryConfig    `yaml:"library"`
	Scan       ScanConfig       `yaml:"scan"`
	Filtering  FilteringConfig  `yaml:"filtering"`
	Thumbnails ThumbnailsConfig `yaml:"thumbnails"`
	Limits     LimitsConfig     `yaml:"limits"`
	Database   DatabaseConfig   `yaml:"database"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type LibraryConfig struct {
	RootPath           string `yaml:"rootPath"`
	IncludeEmptyAlbums bool   `yaml:"includeEmptyAlbums"`
}

type ScanConfig struct {
	DefaultMode         string `yaml:"defaultMode"`
	QuickFallbackToFull bool   `yaml:"quickFallbackToFull"`
	MaxWorkers          int    `yaml:"maxWorkers"`
}

type FilteringConfig struct {
	ExcludePatterns        []string `yaml:"excludePatterns"`
	AllowedImageExtensions []string `yaml:"allowedImageExtensions"`
	AllowedVideoExtensions []string `yaml:"allowedVideoExtensions"`
	EnableMimeSniff        bool     `yaml:"enableMimeSniff"`
}

type ThumbnailsConfig struct {
	CacheDir string `yaml:"cacheDir"`
	Sizes    []int  `yaml:"sizes"`
}

type LimitsConfig struct {
	LargeMediaWarningBytes int64 `yaml:"largeMediaWarningBytes"`
}

type DatabaseConfig struct {
	SQLitePath string `yaml:"sqlitePath"`
}

func (c *Config) Validate() error {
	var errs []string
	if c.Library.RootPath == "" {
		errs = append(errs, "library.rootPath is required")
	}
	if c.Database.SQLitePath == "" {
		errs = append(errs, "database.sqlitePath is required")
	}
	if c.Thumbnails.CacheDir == "" {
		errs = append(errs, "thumbnails.cacheDir is required")
	}
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		errs = append(errs, fmt.Sprintf("server.port %d is invalid (must be 1-65535)", c.Server.Port))
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cfg := defaultConfig()
	if err := yaml.NewDecoder(f).Decode(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{Host: "0.0.0.0", Port: 8080},
		Scan:   ScanConfig{DefaultMode: "quick", QuickFallbackToFull: true, MaxWorkers: 8},
		Limits: LimitsConfig{LargeMediaWarningBytes: 104857600},
		Thumbnails: ThumbnailsConfig{
			Sizes: []int{240, 480, 960},
		},
		Filtering: FilteringConfig{
			ExcludePatterns:        []string{".*", "@eaDir", "Thumbs.db"},
			AllowedImageExtensions: []string{"jpg", "jpeg", "png", "webp", "gif", "heic", "tif", "tiff", "avif"},
			AllowedVideoExtensions: []string{"mp4", "mkv", "mov", "webm", "avi", "m4v"},
			EnableMimeSniff:        true,
		},
	}
}

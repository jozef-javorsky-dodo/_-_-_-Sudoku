package cli

import (
	"fmt"

	"github.com/SUDOKU-ASCII/sudoku/internal/app"
	"github.com/SUDOKU-ASCII/sudoku/internal/config"
	"github.com/SUDOKU-ASCII/sudoku/pkg/obfs/sudoku"
)

func BuildClientConfigsFromLinks(links []string) ([]*config.Config, [][]*sudoku.Table, error) {
	configs := make([]*config.Config, 0, len(links))
	tableSets := make([][]*sudoku.Table, 0, len(links))
	for _, link := range links {
		cfg, err := config.BuildConfigFromShortLink(link)
		if err != nil {
			return nil, nil, fmt.Errorf("parse short link %q: %w", link, err)
		}
		tables, err := app.BuildTables(cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("build table for %s: %w", cfg.ServerAddress, err)
		}
		configs = append(configs, cfg)
		tableSets = append(tableSets, tables)
	}
	return configs, tableSets, nil
}

func LoadConfigs(paths []string) ([]*config.Config, error) {
	configs := make([]*config.Config, 0, len(paths))
	for _, path := range paths {
		cfg, err := config.Load(path)
		if err != nil {
			return nil, fmt.Errorf("load config from %s: %w", path, err)
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}

func BuildClientTableSets(configs []*config.Config) ([][]*sudoku.Table, error) {
	tableSets := make([][]*sudoku.Table, 0, len(configs))
	for _, cfg := range configs {
		if cfg.Mode != "client" {
			return nil, fmt.Errorf("mixed config modes are unsupported when using multiple -c values")
		}
		tables, err := app.BuildTables(cfg)
		if err != nil {
			return nil, fmt.Errorf("build table for %s: %w", cfg.ServerAddress, err)
		}
		tableSets = append(tableSets, tables)
	}
	return tableSets, nil
}

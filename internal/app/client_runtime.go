package app

import (
	"fmt"
	"strings"

	"github.com/SUDOKU-ASCII/sudoku/internal/config"
	"github.com/SUDOKU-ASCII/sudoku/internal/tunnel"
	"github.com/SUDOKU-ASCII/sudoku/pkg/logx"
	"github.com/SUDOKU-ASCII/sudoku/pkg/obfs/sudoku"
)

type clientRuntime struct {
	Config     *config.Config
	Tables     []*sudoku.Table
	BaseDialer tunnel.BaseDialer
	Dialer     tunnel.Dialer
	NodeID     string
}

func buildClientRuntime(cfg *config.Config, tables []*sudoku.Table) (*clientRuntime, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil client config")
	}
	if cfg.Mode != "client" {
		return nil, fmt.Errorf("config mode must be client, got %q", cfg.Mode)
	}
	nodeID := clientNodeID(cfg)

	privateKeyBytes, changed, err := normalizeClientKey(cfg)
	if err != nil {
		return nil, fmt.Errorf("process key for %s: %w", nodeID, err)
	}
	if changed {
		logx.Infof("Init", "Derived Public Key for %s: %s", nodeID, cfg.Key)
	}

	if len(tables) == 0 || changed {
		tables, err = BuildTables(cfg)
		if err != nil {
			return nil, fmt.Errorf("build table(s) for %s: %w", nodeID, err)
		}
	}

	baseDialer := tunnel.BaseDialer{
		Config:     cfg,
		Tables:     tables,
		PrivateKey: privateKeyBytes,
	}

	var dialer tunnel.Dialer
	if cfg.HTTPMaskSessionMuxEnabled() {
		dialer = &tunnel.MuxDialer{BaseDialer: baseDialer}
	} else {
		dialer = &tunnel.StandardDialer{BaseDialer: baseDialer}
	}

	return &clientRuntime{
		Config:     cfg,
		Tables:     tables,
		BaseDialer: baseDialer,
		Dialer:     dialer,
		NodeID:     nodeID,
	}, nil
}

func buildClientRuntimes(configs []*config.Config, tableSets [][]*sudoku.Table) ([]*clientRuntime, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("no client configs provided")
	}

	runtimes := make([]*clientRuntime, 0, len(configs))
	for i, cfg := range configs {
		var tables []*sudoku.Table
		if i < len(tableSets) {
			tables = tableSets[i]
		}
		rt, err := buildClientRuntime(cfg, tables)
		if err != nil {
			return nil, err
		}
		runtimes = append(runtimes, rt)
	}
	return runtimes, nil
}

func buildOutboundDialer(runtimes []*clientRuntime) (tunnel.Dialer, error) {
	if len(runtimes) == 0 {
		return nil, fmt.Errorf("no client runtimes provided")
	}
	if len(runtimes) == 1 {
		rt := runtimes[0]
		if rt.Config.HTTPMaskSessionMuxEnabled() {
			logx.Infof("Init", "Enabled HTTPMask session mux (single tunnel, multi-target)")
		}
		return rt.Dialer, nil
	}

	nodes := make([]tunnel.BalancedNode, 0, len(runtimes))
	muxCount := 0
	for _, rt := range runtimes {
		if rt.Config.HTTPMaskSessionMuxEnabled() {
			muxCount++
		}
		nodes = append(nodes, tunnel.BalancedNode{
			ID:     rt.NodeID,
			Dialer: rt.Dialer,
		})
	}

	if muxCount > 0 {
		logx.Infof("Init", "Enabled HTTPMask session mux on %d/%d outbound node(s)", muxCount, len(runtimes))
	}

	return tunnel.NewBalancedDialer(nodes)
}

func clientNodeID(cfg *config.Config) string {
	if cfg == nil {
		return "client-node"
	}
	serverAddr := strings.TrimSpace(cfg.ServerAddress)
	if serverAddr == "" {
		serverAddr = "unknown-server"
	}
	mode := cfg.HTTPMask.Mode
	if strings.TrimSpace(mode) == "" {
		mode = "legacy"
	}
	return fmt.Sprintf("%s/%s", serverAddr, mode)
}

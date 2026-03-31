package apis

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/SUDOKU-ASCII/sudoku/internal/protocol"
	"github.com/SUDOKU-ASCII/sudoku/internal/tunnel"
	"github.com/SUDOKU-ASCII/sudoku/pkg/obfs/sudoku"
)

func TestDialDirectionalASCIIWithCustomTable(t *testing.T) {
	table, err := sudoku.NewTableWithCustom("directional-seed", "up_ascii_down_entropy", "xpxvvpvv")
	if err != nil {
		t.Fatalf("build directional table: %v", err)
	}

	targetLn := startSingleEchoTarget(t, []byte("ping"), []byte("pong"))
	defer targetLn.Close()

	serverLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen server: %v", err)
	}
	defer serverLn.Close()

	serverCfg := &ProtocolConfig{
		Key:                     "directional-key",
		AEADMethod:              "chacha20-poly1305",
		Table:                   table,
		PaddingMin:              0,
		PaddingMax:              0,
		EnablePureDownlink:      true,
		HandshakeTimeoutSeconds: 5,
		DisableHTTPMask:         true,
	}

	serverErr := startSingleEchoTunnelServer(t, serverLn, serverCfg, []byte("ping"), []byte("pong"))

	clientCfg := &ProtocolConfig{
		ServerAddress:      serverLn.Addr().String(),
		TargetAddress:      targetLn.Addr().String(),
		Key:                "directional-key",
		AEADMethod:         "chacha20-poly1305",
		Table:              table,
		PaddingMin:         0,
		PaddingMax:         0,
		EnablePureDownlink: true,
		DisableHTTPMask:    true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, clientCfg)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	assertConnRoundTrip(t, conn, []byte("ping"), []byte("pong"))
	assertServerFinished(t, serverErr)
}

func TestDialPackedDownlinkWithoutAEAD(t *testing.T) {
	table := sudoku.NewTable("packed-no-aead-seed", "prefer_ascii")

	targetLn := startSingleEchoTarget(t, []byte("ping"), []byte("pong"))
	defer targetLn.Close()

	serverLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen server: %v", err)
	}
	defer serverLn.Close()

	serverCfg := &ProtocolConfig{
		Key:                     "packed-no-aead-key",
		AEADMethod:              "none",
		Table:                   table,
		PaddingMin:              0,
		PaddingMax:              0,
		EnablePureDownlink:      false,
		HandshakeTimeoutSeconds: 5,
		DisableHTTPMask:         true,
	}

	serverErr := startSingleEchoTunnelServer(t, serverLn, serverCfg, []byte("ping"), []byte("pong"))

	clientCfg := &ProtocolConfig{
		ServerAddress:      serverLn.Addr().String(),
		TargetAddress:      targetLn.Addr().String(),
		Key:                "packed-no-aead-key",
		AEADMethod:         "none",
		Table:              table,
		PaddingMin:         0,
		PaddingMax:         0,
		EnablePureDownlink: false,
		DisableHTTPMask:    true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, clientCfg)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	assertConnRoundTrip(t, conn, []byte("ping"), []byte("pong"))
	assertServerFinished(t, serverErr)
}

func TestDialDirectionalASCIIWithCustomTableRotationHint(t *testing.T) {
	serverTables := make([]*sudoku.Table, 0, 2)
	for _, pattern := range []string{"xpxvvpvv", "vxpvxvvp"} {
		table, err := sudoku.NewTableWithCustom("directional-rotate-seed", "up_ascii_down_entropy", pattern)
		if err != nil {
			t.Fatalf("build directional table: %v", err)
		}
		serverTables = append(serverTables, table)
	}
	clientTables := []*sudoku.Table{serverTables[1], serverTables[0]}

	targetLn := startSingleEchoTarget(t, []byte("ping"), []byte("pong"))
	defer targetLn.Close()

	serverLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen server: %v", err)
	}
	defer serverLn.Close()

	serverCfg := &ProtocolConfig{
		Key:                     "directional-rotate-key",
		AEADMethod:              "chacha20-poly1305",
		Tables:                  serverTables,
		PaddingMin:              0,
		PaddingMax:              0,
		EnablePureDownlink:      true,
		HandshakeTimeoutSeconds: 5,
		DisableHTTPMask:         true,
	}

	serverErr := startSingleEchoTunnelServer(t, serverLn, serverCfg, []byte("ping"), []byte("pong"))

	rawConn, err := net.DialTimeout("tcp", serverLn.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatalf("dial server: %v", err)
	}
	defer rawConn.Close()

	clientCfg := &ProtocolConfig{
		ServerAddress:      serverLn.Addr().String(),
		TargetAddress:      targetLn.Addr().String(),
		Key:                "directional-rotate-key",
		AEADMethod:         "chacha20-poly1305",
		Tables:             clientTables,
		PaddingMin:         0,
		PaddingMax:         0,
		EnablePureDownlink: true,
		DisableHTTPMask:    true,
	}
	seed := canonicalCryptoSeedKey(clientCfg.Key)

	conn, err := upgradeClientConnWithUplinkMode(rawConn, clientCfg, clientTables[0], clientTables[0].Hint(), seed, tunnel.ObfsUplinkPure, func(conn net.Conn) error {
		return tunnel.WriteKIPMessage(conn, tunnel.KIPTypeOpenTCP, mustEncodeAddress(t, targetLn.Addr().String()))
	}, true)
	if err != nil {
		t.Fatalf("dial with hinted table failed: %v", err)
	}
	defer conn.Close()

	assertConnRoundTrip(t, conn, []byte("ping"), []byte("pong"))
	assertServerFinished(t, serverErr)
}

func mustEncodeAddress(t *testing.T, addr string) []byte {
	t.Helper()

	var payload []byte
	buf := bytes.NewBuffer(payload)
	if err := protocol.WriteAddress(buf, addr); err != nil {
		t.Fatalf("encode address: %v", err)
	}
	return buf.Bytes()
}

func startSingleEchoTarget(t *testing.T, req, resp []byte) net.Listener {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen target: %v", err)
	}

	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()

		buf := make([]byte, len(req))
		if _, err := io.ReadFull(c, buf); err != nil {
			return
		}
		if !bytes.Equal(buf, req) {
			return
		}
		_, _ = c.Write(resp)
	}()

	return ln
}

func startSingleEchoTunnelServer(t *testing.T, serverLn net.Listener, serverCfg *ProtocolConfig, req, resp []byte) <-chan error {
	t.Helper()

	serverErr := make(chan error, 1)
	go func() {
		raw, err := serverLn.Accept()
		if err != nil {
			serverErr <- err
			return
		}

		conn, targetAddr, err := ServerHandshake(raw, serverCfg)
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()

		targetConn, err := net.DialTimeout("tcp", targetAddr, 5*time.Second)
		if err != nil {
			serverErr <- err
			return
		}
		defer targetConn.Close()

		buf := make([]byte, len(req))
		if _, err := io.ReadFull(conn, buf); err != nil {
			serverErr <- err
			return
		}
		if !bytes.Equal(buf, req) {
			serverErr <- io.ErrUnexpectedEOF
			return
		}
		if _, err := targetConn.Write(buf); err != nil {
			serverErr <- err
			return
		}

		reply := make([]byte, len(resp))
		if _, err := io.ReadFull(targetConn, reply); err != nil {
			serverErr <- err
			return
		}
		_, err = conn.Write(reply)
		serverErr <- err
	}()
	return serverErr
}

func assertConnRoundTrip(t *testing.T, conn net.Conn, req, resp []byte) {
	t.Helper()

	if _, err := conn.Write(req); err != nil {
		t.Fatalf("write request: %v", err)
	}

	buf := make([]byte, len(resp))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read response: %v", err)
	}
	if !bytes.Equal(buf, resp) {
		t.Fatalf("unexpected response: %q", string(buf))
	}
}

func assertServerFinished(t *testing.T, serverErr <-chan error) {
	t.Helper()

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("server: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("server timeout")
	}
}

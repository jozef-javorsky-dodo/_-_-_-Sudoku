package apis

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/SUDOKU-ASCII/sudoku/pkg/obfs/sudoku"
)

func TestDialDirectionalASCIIWithCustomTable(t *testing.T) {
	table, err := sudoku.NewTableWithCustom("directional-seed", "up_ascii_down_entropy", "xpxvvpvv")
	if err != nil {
		t.Fatalf("build directional table: %v", err)
	}

	targetLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen target: %v", err)
	}
	defer targetLn.Close()

	go func() {
		c, err := targetLn.Accept()
		if err != nil {
			return
		}
		defer c.Close()

		buf := make([]byte, 4)
		if _, err := io.ReadFull(c, buf); err != nil {
			return
		}
		if string(buf) != "ping" {
			return
		}
		_, _ = c.Write([]byte("pong"))
	}()

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

		buf := make([]byte, 4)
		if _, err := io.ReadFull(conn, buf); err != nil {
			serverErr <- err
			return
		}
		if string(buf) != "ping" {
			serverErr <- io.ErrUnexpectedEOF
			return
		}
		if _, err := targetConn.Write(buf); err != nil {
			serverErr <- err
			return
		}
		if _, err := io.ReadFull(targetConn, buf); err != nil {
			serverErr <- err
			return
		}
		_, err = conn.Write(buf)
		serverErr <- err
	}()

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

	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read pong: %v", err)
	}
	if string(buf) != "pong" {
		t.Fatalf("unexpected response: %q", string(buf))
	}

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("server: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("server timeout")
	}
}

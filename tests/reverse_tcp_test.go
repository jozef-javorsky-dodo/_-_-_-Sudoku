/*
Copyright (C) 2026 by saba <contact me via issue>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program. If not, see <http://www.gnu.org/licenses/>.

In addition, no derivative work may use the name or imply association
with this application without prior consent.
*/
package tests

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/saba-futai/sudoku/internal/config"
	"golang.org/x/crypto/ssh"
)

func TestReverseProxy_TCP_DefaultRoute(t *testing.T) {
	originLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen origin: %v", err)
	}
	defer originLn.Close()

	originAddr := originLn.Addr().String()
	go func() {
		for {
			c, err := originLn.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}(c)
		}
	}()

	serverKey, clientKey := newTestKeys(t)

	ports, err := getFreePorts(3)
	if err != nil {
		t.Fatalf("ports: %v", err)
	}
	serverPort := ports[0]
	clientPort := ports[1]
	reversePort := ports[2]

	reverseListen := localServerAddr(reversePort)

	serverCfg := newTestServerConfig(serverPort, serverKey)
	serverCfg.Reverse = &config.ReverseConfig{Listen: reverseListen}
	startSudokuServer(t, serverCfg)
	waitForAddr(t, reverseListen)

	clientCfg := newTestClientConfig(clientPort, localServerAddr(serverPort), clientKey)
	clientCfg.Reverse = &config.ReverseConfig{
		ClientID: "r4s",
		Routes: []config.ReverseRoute{
			// Path empty => raw TCP reverse on reverse.listen.
			{Target: originAddr},
		},
	}
	startSudokuClient(t, clientCfg)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", reverseListen, 200*time.Millisecond)
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		_ = c.SetDeadline(time.Now().Add(2 * time.Second))
		if _, err := c.Write([]byte("ping")); err != nil {
			_ = c.Close()
			time.Sleep(50 * time.Millisecond)
			continue
		}
		buf := make([]byte, 4)
		_, err = io.ReadFull(c, buf)
		_ = c.Close()
		if err == nil && string(buf) == "ping" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("reverse tcp proxy did not become ready")
}

func TestReverseProxy_TCP_DefaultRoute_SSH(t *testing.T) {
	sshSrv := startTestSSHServer(t, "127.0.0.1:0", "u", "p")
	defer sshSrv.Close()

	serverKey, clientKey := newTestKeys(t)

	ports, err := getFreePorts(3)
	if err != nil {
		t.Fatalf("ports: %v", err)
	}
	serverPort := ports[0]
	clientPort := ports[1]
	reversePort := ports[2]

	reverseListen := localServerAddr(reversePort)

	serverCfg := newTestServerConfig(serverPort, serverKey)
	serverCfg.Reverse = &config.ReverseConfig{Listen: reverseListen}
	startSudokuServer(t, serverCfg)
	waitForAddr(t, reverseListen)

	clientCfg := newTestClientConfig(clientPort, localServerAddr(serverPort), clientKey)
	clientCfg.Reverse = &config.ReverseConfig{
		ClientID: "r4s",
		Routes: []config.ReverseRoute{
			// Path empty => raw TCP reverse on reverse.listen.
			{Target: sshSrv.Addr()},
		},
	}
	startSudokuClient(t, clientCfg)

	rawConn, err := net.DialTimeout("tcp", reverseListen, 5*time.Second)
	if err != nil {
		t.Fatalf("tcp dial: %v", err)
	}
	_ = rawConn.SetDeadline(time.Now().Add(10 * time.Second))

	sshCfg := &ssh.ClientConfig{
		User:            "u",
		Auth:            []ssh.AuthMethod{ssh.Password("p")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	cconn, chans, reqs, err := ssh.NewClientConn(rawConn, reverseListen, sshCfg)
	if err != nil {
		_ = rawConn.Close()
		t.Fatalf("ssh handshake: %v", err)
	}
	_ = rawConn.SetDeadline(time.Time{})

	client := ssh.NewClient(cconn, chans, reqs)
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		t.Fatalf("ssh session: %v", err)
	}
	defer sess.Close()

	out, err := sess.CombinedOutput("echo hello")
	if err != nil {
		t.Fatalf("ssh exec: %v (out=%q)", err, string(out))
	}
	if string(out) != "echo hello" {
		t.Fatalf("unexpected output: %q", string(out))
	}
}

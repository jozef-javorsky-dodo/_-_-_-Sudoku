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
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type testSSHServer struct {
	ln        net.Listener
	waitGroup sync.WaitGroup
	closeOnce sync.Once
}

func startTestSSHServer(t testing.TB, listenAddr, username, password string) *testSSHServer {
	t.Helper()

	debugf := func(format string, args ...any) {
		if testing.Verbose() {
			t.Logf(format, args...)
		}
	}

	_, hostKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ssh host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(hostKey)
	if err != nil {
		t.Fatalf("ssh signer: %v", err)
	}

	cfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == username && string(pass) == password {
				return nil, nil
			}
			return nil, errors.New("unauthorized")
		},
	}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		t.Fatalf("ssh listen: %v", err)
	}

	s := &testSSHServer{ln: ln}
	s.waitGroup.Add(1)
	go func() {
		defer s.waitGroup.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			s.waitGroup.Add(1)
			go func(conn net.Conn) {
				defer s.waitGroup.Done()
				defer conn.Close()

				sshConn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
				if err != nil {
					debugf("[ssh-srv] handshake failed: %v", err)
					return
				}
				debugf("[ssh-srv] conn established: user=%q remote=%s", sshConn.User(), sshConn.RemoteAddr())
				defer sshConn.Close()
				go ssh.DiscardRequests(reqs)

				for ch := range chans {
					if ch.ChannelType() != "session" {
						_ = ch.Reject(ssh.UnknownChannelType, "unknown")
						continue
					}
					channel, requests, err := ch.Accept()
					if err != nil {
						continue
					}
					debugf("[ssh-srv] session channel accepted")
					s.waitGroup.Add(1)
					go func(ch ssh.Channel, in <-chan *ssh.Request) {
						defer s.waitGroup.Done()
						defer ch.Close()

						for req := range in {
							switch req.Type {
							case "exec":
								// Payload is a SSH-encoded string of the command.
								var payload struct{ Command string }
								if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
									_ = req.Reply(false, nil)
									continue
								}
								debugf("[ssh-srv] exec: %q", payload.Command)
								_ = req.Reply(true, nil)

								// Echo back the command to prove exec worked.
								_, _ = io.Copy(ch, bytes.NewBufferString(payload.Command))

								debugf("[ssh-srv] sending exit-status")
								_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: 0}))
								debugf("[ssh-srv] exit-status sent, closing channel")
								return
							default:
								_ = req.Reply(false, nil)
							}
						}
					}(channel, requests)
				}
			}(c)
		}
	}()

	return s
}

func TestTestSSHServer_DirectExec(t *testing.T) {
	srv := startTestSSHServer(t, "127.0.0.1:0", "u", "p")
	defer srv.Close()

	rawConn, err := net.DialTimeout("tcp", srv.Addr(), 5*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = rawConn.SetDeadline(time.Now().Add(10 * time.Second))

	cfg := &ssh.ClientConfig{
		User:            "u",
		Auth:            []ssh.AuthMethod{ssh.Password("p")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	cconn, chans, reqs, err := ssh.NewClientConn(rawConn, srv.Addr(), cfg)
	if err != nil {
		_ = rawConn.Close()
		t.Fatalf("handshake: %v", err)
	}
	_ = rawConn.SetDeadline(time.Time{})

	client := ssh.NewClient(cconn, chans, reqs)
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	defer sess.Close()

	out, err := sess.CombinedOutput("echo hello")
	if err != nil {
		t.Fatalf("exec: %v (out=%q)", err, string(out))
	}
	if string(out) != "echo hello" {
		t.Fatalf("unexpected output: %q", string(out))
	}
}

func (s *testSSHServer) Addr() string {
	return s.ln.Addr().String()
}

func (s *testSSHServer) Close() error {
	s.closeOnce.Do(func() {
		_ = s.ln.Close()
	})
	s.waitGroup.Wait()
	return nil
}

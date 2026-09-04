package console

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/stretchr/testify/require"
	ssh "github.com/tailscale/gliderssh"
	gossh "golang.org/x/crypto/ssh"
)

// mockMetal is a metal adapter that records the token and machine it is given
// and returns a fixed, valid machine so sessionHandler can proceed.
type mockMetal struct {
	token   string
	project string

	machineID atomic.Value // string
	calls     atomic.Int32
}

func (m *mockMetal) getMachine(_ context.Context, machineID string) (*machine, error) {
	m.calls.Add(1)
	m.machineID.Store(machineID)
	return &machine{
		id:                        machineID,
		role:                      apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
		allocated:                 true,
		managementServerAddresses: []string{"machine.example.internal:2222"},
		sshPublicKeys:             nil,
		createdAt:                 time.Now(),
	}, nil
}

func (m *mockMetal) checkIsAuthenticated(context.Context) (bool, error) {
	return false, nil
}

// startMachineSSHServer runs a minimal ssh server that stands in for the
// machine's serial console. It accepts any connection, answers pty/env/exec
// requests and echoes stdin back to stdout.
func startMachineSSHServer(t *testing.T) (addr string, cleanup func()) {
	t.Helper()

	hostKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	hostSigner, err := gossh.NewSignerFromKey(hostKey)
	require.NoError(t, err)

	config := &gossh.ServerConfig{
		PublicKeyCallback: func(gossh.ConnMetadata, gossh.PublicKey) (*gossh.Permissions, error) {
			return &gossh.Permissions{}, nil
		},
		PasswordCallback: func(gossh.ConnMetadata, []byte) (*gossh.Permissions, error) {
			return &gossh.Permissions{}, nil
		},
	}
	config.AddHostKey(hostSigner)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go func() {
		for {
			nConn, err := l.Accept()
			if err != nil {
				return
			}
			go func(n net.Conn) {
				_, chans, reqs, err := gossh.NewServerConn(n, config)
				if err != nil {
					_ = n.Close()
					return
				}
				go gossh.DiscardRequests(reqs)
				for newCh := range chans {
					if newCh.ChannelType() != "session" {
						_ = newCh.Reject(gossh.UnknownChannelType, "unsupported channel")
						continue
					}
					ch, requests, err := newCh.Accept()
					if err != nil {
						continue
					}
					go func(in <-chan *gossh.Request) {
						for req := range in {
							ok := req.Type == "pty-req" || req.Type == "shell" ||
								req.Type == "exec" || req.Type == "env"
							_ = req.Reply(ok, nil)
						}
					}(requests)
					go func() {
						_, _ = io.Copy(ch, ch) // echo
						_, _ = ch.SendRequest("exit-status", false, gossh.Marshal(struct{ Status uint32 }{0}))
						_ = ch.Close()
					}()
				}
			}(nConn)
		}
	}()

	return l.Addr().String(), func() { _ = l.Close() }
}

// consoleSSHServer starts a metal-console-style ssh.Server on a local listener
// (same handlers as Run) and returns its address.
func consoleSSHServer(t *testing.T, cs *consoleServer) (addr string, cleanup func()) {
	t.Helper()

	hostKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	hostSigner, err := gossh.NewSignerFromKey(hostKey)
	require.NoError(t, err)

	s := &ssh.Server{
		Handler:          cs.sessionHandler,
		PublicKeyHandler: cs.noopPublicKeyHandler,
		Banner:           "metal-stack.io console server\n",
	}
	s.AddHostKey(hostSigner)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go func() {
		_ = s.Serve(l)
	}()

	return l.Addr().String(), func() { _ = l.Close(); _ = s.Close() }
}

func TestSSHSessionWithPrivateKeyAndTokenEnv(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cs := NewServer(logger, &Specification{})

	mock := &mockMetal{}
	cs.newMetal = func(_ *slog.Logger, token, project string, _ Specification) (metal, error) {
		mock.token = token
		mock.project = project
		return mock, nil
	}

	machineAddr, machineCleanup := startMachineSSHServer(t)
	defer machineCleanup()

	cs.connectMachine = func(_ string, machineID string) (func(), *gossh.Session, error) {
		client, err := gossh.Dial("tcp", machineAddr, &gossh.ClientConfig{
			User:            machineID,
			Auth:            []gossh.AuthMethod{gossh.Password("x")},
			HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		})
		if err != nil {
			return nil, nil, err
		}
		session, err := client.NewSession()
		if err != nil {
			_ = client.Close()
			return nil, nil, err
		}
		return func() {
			_ = session.Close()
			_ = client.Close()
		}, session, nil
	}

	consoleAddr, consoleCleanup := consoleSSHServer(t, cs)
	defer consoleCleanup()

	// Client: authenticate with a private key and forward the oidc token and
	// project via ssh environment variables.
	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	clientSigner, err := gossh.NewSignerFromKey(clientKey)
	require.NoError(t, err)

	client, err := gossh.Dial("tcp", consoleAddr, &gossh.ClientConfig{
		User:            "test-machine-uuid",
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(clientSigner)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	})
	require.NoError(t, err)
	defer func() {
		_ = client.Close()
	}()

	session, err := client.NewSession()
	require.NoError(t, err)
	defer func() {
		_ = session.Close()
	}()

	require.NoError(t, session.Setenv("LC_METAL_STACK_OIDC_TOKEN", "test-oidc-token"))
	require.NoError(t, session.Setenv("LC_METAL_STACK_PROJECT", "test-project"))
	require.NoError(t, session.Setenv("LC_METAL_STACK_ISADMIN", "true"))

	var stdout bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = io.Discard
	session.Stdin = strings.NewReader("")

	runDone := make(chan error, 1)
	go func() {
		runDone <- session.Run("")
	}()

	// sessionHandler should receive the forwarded token/project env vars and
	// call the mock metal's getMachine with the ssh username.
	require.Eventually(t, func() bool {
		return mock.calls.Load() >= 1
	}, 5*time.Second, 10*time.Millisecond)

	require.Equal(t, "test-oidc-token", mock.token)
	require.Equal(t, "test-project", mock.project)
	require.Equal(t, "test-machine-uuid", mock.machineID.Load())

	// Closing the client session unblocks the io redirection in sessionHandler
	// so it can return. The client-side session may report an exit error here
	// because the session was closed mid-flight; that is expected and not part
	// of what this test asserts.
	_ = session.Close()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("session.Run did not return after closing the session")
	}
}

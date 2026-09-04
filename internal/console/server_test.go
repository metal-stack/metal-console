package console

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	ssh "github.com/tailscale/gliderssh"
	gossh "golang.org/x/crypto/ssh"
)

type mockSession struct {
	ssh.Session
	env []string
}

func (m *mockSession) Environ() []string { return m.env }

func TestLoadPublicHostKey(t *testing.T) {
	// given
	pubHostKey := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCzH+R+UhjVicUtI0daNUcedYhfvgT1dbZXgY33Ibm4MOo+X84Iwuzirm3QFnYf2O3uyZjNyrA6fj9qFE7Ekul4bD6PCstQupXPwfPMjns2M7tkHsKnLYjNxWNql/rCUxoH2B6nPyztcRCass3lIc2clfXkCY9Jtf7kgC2e/dmchywPV5PrFqtlHgZUnyoPyWBH7OjPLVxYwtCJn96sFkrjaG9QDOeoeiNvcGlk4DJp/g9L4f2AaEq69x8+gBTFUqAFsD8ecO941cM8sa1167rsRPx7SK3270Ji5EUF3lZsgpaiIgMhtIB/7QNTkN9ZjQBazxxlNVN6WthF8okb7OSt"

	// when
	_, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pubHostKey))

	// then
	require.NoError(t, err)
}

func TestTokenAndProjectFromSessionEnv(t *testing.T) {
	tests := []struct {
		name      string
		env       []string
		wantToken string
		wantProj  string
	}{
		{
			name: "all values set",
			env: []string{
				"LC_METAL_STACK_OIDC_TOKEN=abc123",
				"LC_METAL_STACK_PROJECT=proj-1",
				"PATH=/usr/bin",
			},
			wantToken: "abc123",
			wantProj:  "proj-1",
		},
		{
			name:      "only token",
			env:       []string{"LC_METAL_STACK_OIDC_TOKEN=xyz"},
			wantToken: "xyz",
		},
		{
			name:     "only project",
			env:      []string{"LC_METAL_STACK_PROJECT=p-2"},
			wantProj: "p-2",
		},
		{
			name: "empty env",
			env:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &mockSession{env: tt.env}
			token, project := tokenAndProjectFromSessionEnv(s)
			require.Equal(t, tt.wantToken, token)
			require.Equal(t, tt.wantProj, project)
		})
	}
}

func newTestPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	signer, err := gossh.NewSignerFromKey(priv)
	require.NoError(t, err)
	return signer.PublicKey()
}

func TestCheckAuthorizedKeys(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cs := NewServer(logger, &Specification{})

	pub := newTestPublicKey(t)
	authorized := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(pub)))

	m := machine{
		id:            "m-1",
		sshPublicKeys: []string{authorized},
	}

	t.Run("matching key is accepted", func(t *testing.T) {
		require.NoError(t, cs.checkAuthorizedKeys(m, pub))
	})

	t.Run("non matching key is rejected", func(t *testing.T) {
		other := newTestPublicKey(t)
		require.Error(t, cs.checkAuthorizedKeys(m, other))
	})

	t.Run("no keys on machine is rejected", func(t *testing.T) {
		empty := machine{id: "m-2"}
		require.Error(t, cs.checkAuthorizedKeys(empty, pub))
	})

	t.Run("invalid stored key is rejected", func(t *testing.T) {
		invalid := machine{id: "m-3", sshPublicKeys: []string{"not-a-valid-ssh-key"}}
		require.Error(t, cs.checkAuthorizedKeys(invalid, pub))
	})
}

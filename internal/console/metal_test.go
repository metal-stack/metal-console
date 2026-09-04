package console

import (
	"io"
	"log/slog"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func mustToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret"))
	require.NoError(t, err)
	return tok
}

func TestIsV2TokenType(t *testing.T) {
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name    string
		token   string
		want    bool
		wantErr bool
	}{
		{
			name:  "v2 api token",
			token: mustToken(t, jwt.MapClaims{"type": "TOKEN_TYPE_API"}),
			want:  true,
		},
		{
			name:  "v2 user token",
			token: mustToken(t, jwt.MapClaims{"type": "TOKEN_TYPE_USER"}),
			want:  true,
		},
		{
			name:  "v1 token with roles",
			token: mustToken(t, jwt.MapClaims{"roles": []string{"role-a"}}),
			want:  false,
		},
		{
			name:    "no recognizable claim",
			token:   mustToken(t, jwt.MapClaims{"foo": "bar"}),
			wantErr: true,
		},
		{
			name:    "unparsable token",
			token:   "not-a-valid-jwt",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := isV2TokenType(discard, tt.token)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

package console

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

const (
	// oidcTokenEnv environment variable passed through ssh to forward the token
	oidcTokenEnv = "LC_METAL_STACK_OIDC_TOKEN"
	// projectEnv environment variable passed through ssh to forward the metal project
	projectEnv = "LC_METAL_STACK_PROJECT"
)

type (
	machine struct {
		id                        string
		role                      apiv2.MachineAllocationType
		allocated                 bool
		managementServerAddresses []string
		sshPublicKeys             []string
		createdAt                 time.Time
	}
)

type metal interface {
	getMachine(ctx context.Context, machineID string) (*machine, error)
	// checkIsAuthenticated returns true if the user is admin, an error is returned of authentication failed
	checkIsAuthenticated(context.Context) (bool, error)
}

func newMetal(log *slog.Logger, token, project string, spec Specification) (metal, error) {
	isV2Token, err := isV2TokenType(log, token)
	if err != nil {
		return nil, err
	}

	if isV2Token {
		return newV2(log, spec.MetalAPIServerURL, token, project)
	} else {
		return newV1(log, spec.MetalAPIURL, token, spec.AdminGroupName)
	}
}

func isV2TokenType(log *slog.Logger, token string) (bool, error) {
	claims := &jwt.MapClaims{}
	parser := jwt.NewParser()
	// using parseUnverified because result is not used for authorization only for inspection
	_, _, err := parser.ParseUnverified(token, claims)
	if err != nil {
		return false, err
	}

	log.Debug("isV2Token", "token", claims)

	// APIv2 Token must contain either:
	//  "type": "TOKEN_TYPE_API"
	//  "type": "TOKEN_TYPE_USER"

	// APIv1 Token must contain a "roles" slice
	for k, v := range *claims {
		switch k {
		case "type":
			if v == apiv2.TokenType_TOKEN_TYPE_API.String() || v == apiv2.TokenType_TOKEN_TYPE_USER.String() {
				return true, nil
			}
		case "roles":
			return false, nil
		}
	}

	return false, fmt.Errorf("unable to detect token api version from claims: %v", claims)
}

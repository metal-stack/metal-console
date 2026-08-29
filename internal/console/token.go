package console

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

func (cs *consoleServer) isV2TokenType(token string) (bool, error) {
	claims := &jwt.MapClaims{}
	parser := jwt.NewParser()
	_, _, err := parser.ParseUnverified(token, claims)
	if err != nil {
		return false, err
	}

	cs.log.Info("isV2Token", "token", claims)

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

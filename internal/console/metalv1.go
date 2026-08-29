package console

import (
	"fmt"
	"slices"

	metalgo "github.com/metal-stack/metal-go"
	"github.com/metal-stack/metal-go/api/client/user"
	"github.com/metal-stack/metal-go/api/models"
)

func (cs *consoleServer) checkIsAuthenticatedUserV1(token string) (*models.V1User, error) {
	if token == "" {
		return nil, fmt.Errorf("unable to find OIDC token stored in %s env variable which is required for machine console access", oidcEnv)
	}

	metal, err := metalgo.NewDriver(cs.spec.MetalAPIURL, token, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create metal client: %w", err)
	}

	user, err := metal.User().GetMe(user.NewGetMeParams(), nil)
	if err != nil {
		cs.log.Error("failed to fetch user details from oidc token", "error", err, "token", token)
		return nil, fmt.Errorf("given oidc token is invalid")
	}

	return user.Payload, nil
}

func (cs *consoleServer) checkIsAdminV1(token string) error {
	user, err := cs.checkIsAuthenticatedUserV1(token)
	if err != nil {
		return err
	}

	if !slices.Contains(user.Groups, cs.spec.AdminGroupName) {
		return fmt.Errorf("you are not member of required admin group:%s to access this machine console", cs.spec.AdminGroupName)
	}

	return nil
}

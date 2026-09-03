package console

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	metalgo "github.com/metal-stack/metal-go"
	metalmachine "github.com/metal-stack/metal-go/api/client/machine"
	"github.com/metal-stack/metal-go/api/client/user"
	"github.com/metal-stack/metal-go/api/models"
	"github.com/metal-stack/metal-lib/pkg/pointer"
)

type metalv1 struct {
	log            *slog.Logger
	client         metalgo.Client
	adminGroupName string
	token          string
	isadmin        bool
}

func newV1(log *slog.Logger, metalapiv1Url, token, adminGroupName string, isadmin bool) (metal, error) {
	if token == "" {
		return nil, fmt.Errorf("unable to find OIDC token stored in %s env variable which is required for machine console access", oidcTokenEnv)
	}

	metal, err := metalgo.NewDriver(metalapiv1Url, token, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create metal client: %w", err)
	}
	return &metalv1{
		log:            log,
		client:         metal,
		token:          token,
		adminGroupName: adminGroupName,
		isadmin:        isadmin,
	}, nil
}

func (m *metalv1) getMachine(ctx context.Context, machineID string) (*machine, error) {
	resp, err := m.client.Machine().FindMachine(metalmachine.NewFindMachineParams().WithID(machineID).WithContext(ctx), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch requested machine %s %w", machineID, err)
	}

	var role apiv2.MachineAllocationType
	if resp.Payload != nil && resp.Payload.Allocation != nil && resp.Payload.Allocation.Role != nil {
		switch *resp.Payload.Allocation.Role {
		case models.V1MachineAllocationRoleMachine:
			role = apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE
		case models.V1MachineAllocationRoleFirewall:
			role = apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL
		}
	}

	return &machine{
		id:                        *resp.Payload.ID,
		role:                      role,
		allocated:                 resp.Payload.Allocation != nil,
		managementServerAddresses: []string{resp.Payload.Partition.Mgmtserviceaddress},
		sshPublicKeys:             pointer.SafeDeref(resp.Payload.Allocation).SSHPubKeys,
		createdAt:                 time.Time(pointer.SafeDeref(pointer.SafeDeref(resp.Payload.Allocation).Created)),
	}, nil
}

func (m *metalv1) checkIsAuthenticated(ctx context.Context) (bool, error) {

	user, err := m.client.User().GetMe(user.NewGetMeParams().WithContext(ctx), nil)
	if err != nil {
		m.log.Error("failed to fetch user details from oidc token", "error", err, "token", m.token)
		return false, fmt.Errorf("given oidc token is invalid")
	}

	if slices.Contains(user.Payload.Groups, m.adminGroupName) {
		return true, nil
	}

	return false, nil
}

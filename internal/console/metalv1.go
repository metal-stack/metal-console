package console

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-console/api"
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
}

func newV1(log *slog.Logger, metalapiv1Url, token, adminGroupName string) (metal, error) {
	if token == "" {
		return nil, fmt.Errorf("unable to find OIDC token stored in %s env variable which is required for machine console access", api.OidcTokenEnv)
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
	}, nil
}

func (m *metalv1) getMachine(ctx context.Context, machineID string) (*machine, error) {
	resp, err := m.client.Machine().FindMachine(metalmachine.NewFindMachineParams().WithID(machineID).WithContext(ctx), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch requested machine: %s %w", machineID, err)
	}

	if resp.Payload == nil {
		return nil, fmt.Errorf("machine is nil")
	}

	var (
		ms            = resp.Payload
		createdAt     = time.Time(pointer.SafeDeref(pointer.SafeDeref(ms.Allocation).Created))
		isProvisioned = false
		role          apiv2.MachineAllocationType
	)

	if ms.Allocation != nil && ms.Allocation.Role != nil {
		switch *resp.Payload.Allocation.Role {
		case models.V1MachineAllocationRoleMachine:
			role = apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE
		case models.V1MachineAllocationRoleFirewall:
			role = apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL
		}
	}

	events := slices.DeleteFunc(ms.Events.Log, func(l *models.V1MachineProvisioningEvent) bool {
		return time.Time(l.Time).Before(createdAt)
	})
	for _, event := range events {
		if pointer.SafeDeref(event.Event) == "Phoned Home" {
			isProvisioned = true
			break
		}
	}

	return &machine{
		id:                        pointer.SafeDeref(ms.ID),
		role:                      role,
		allocated:                 ms.Allocation != nil,
		managementServerAddresses: []string{pointer.SafeDeref(ms.Partition).Mgmtserviceaddress},
		sshPublicKeys:             pointer.SafeDeref(ms.Allocation).SSHPubKeys,
		createdAt:                 createdAt,
		isProvisioned:             isProvisioned,
	}, nil
}

func (m *metalv1) checkIsAdmin(ctx context.Context) (bool, error) {
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

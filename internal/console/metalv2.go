package console

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/metal-stack/api/go/client"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-lib/pkg/pointer"
)

type metalv2 struct {
	log     *slog.Logger
	client  client.Client
	token   string
	project string
	isadmin bool
}

func newV2(log *slog.Logger, baseUrl, token, project string, isadmin bool) (metal, error) {
	if token == "" {
		return nil, fmt.Errorf("unable to find OIDC token stored in %s env variable which is required for machine console access", oidcTokenEnv)
	}

	client, err := client.New(&client.DialConfig{
		BaseURL: baseUrl,
		Token:   token,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create metal-apiserver client: %w", err)
	}
	return &metalv2{
		log:     log,
		client:  client,
		token:   token,
		project: project,
		isadmin: isadmin,
	}, nil
}

func (m *metalv2) getMachine(ctx context.Context, machineID string) (*machine, error) {
	var ms *apiv2.Machine
	if m.project == "" || m.isadmin {
		resp, err := m.client.Adminv2().Machine().Get(ctx, &adminv2.MachineServiceGetRequest{
			Uuid: machineID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch requested machine %s %w", machineID, err)
		}
		ms = resp.Machine
	} else {
		resp, err := m.client.Apiv2().Machine().Get(ctx, &apiv2.MachineServiceGetRequest{
			Uuid:    machineID,
			Project: m.project,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch requested machine %s %w", machineID, err)
		}
		ms = resp.Machine
	}
	return &machine{
		id:                        ms.Uuid,
		role:                      pointer.SafeDeref(ms.Allocation).AllocationType,
		allocated:                 ms.Allocation != nil,
		managementServerAddresses: pointer.SafeDeref(ms.Partition).MgmtServiceAddresses,
		sshPublicKeys:             pointer.SafeDeref(ms.Allocation).SshPublicKeys,
		createdAt:                 pointer.SafeDeref(pointer.SafeDeref(ms.Allocation).Meta).CreatedAt.AsTime(),
	}, nil
}

func (m *metalv2) checkIsAuthenticated(ctx context.Context) (bool, error) {
	resp, err := m.client.Apiv2().Method().TokenScopedList(ctx, &apiv2.MethodServiceTokenScopedListRequest{})
	if err != nil {
		m.log.Error("failed to fetch user details from oidc token", "error", err)
		return false, err
	}

	if pointer.SafeDeref(resp.AdminRole) == apiv2.AdminRole_ADMIN_ROLE_EDITOR {
		return true, nil
	}

	return false, nil
}

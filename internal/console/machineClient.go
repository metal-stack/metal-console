package console

import (
	"fmt"

	metalgo "github.com/metal-stack/metal-go"
	"github.com/metal-stack/metal-go/api/client/machine"
	"github.com/metal-stack/metal-go/api/models"
)

func newClient(metalAPIURL string, hmac string) (metalgo.Client, error) {
	client, _, err := metalgo.NewDriver(metalAPIURL, "", hmac)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (cs *consoleServer) getMachine(machineID string) (*models.V1MachineResponse, error) {
	resp, err := cs.client.Machine().FindMachine(machine.NewFindMachineParams().WithID(machineID), nil)
	if err != nil {
		return nil, err
	}
	if resp.Payload == nil {
		return nil, fmt.Errorf("no machine found with ID %q", machineID)
	}
	return resp.Payload, nil
}

func (cs *consoleServer) getMachineIPMI(machineID string) (*models.V1MachineIPMIResponse, error) {
	resp, err := cs.client.Machine().FindIPMIMachine(machine.NewFindIPMIMachineParams().WithID(machineID), nil)
	if err != nil {
		return nil, err
	}
	return resp.Payload, nil
}

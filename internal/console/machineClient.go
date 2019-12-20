package console

import (
	"fmt"
	metalgo "github.com/metal-pod/metal-go"
	"github.com/metal-pod/metal-go/api/models"
)

func newMachineClient(metalAPIURL string, hmac string) (*metalgo.Driver, error) {
	driver, err := metalgo.NewDriver(metalAPIURL, "", hmac)
	if err != nil {
		return nil, err
	}
	return driver, nil
}

func (cs *consoleServer) getMachine(machineID string) (*models.V1MachineResponse, error) {
	mfr := &metalgo.MachineFindRequest{
		ID: &machineID,
	}
	resp, err := cs.machineClient.MachineFind(mfr)
	if err != nil {
		return nil, err
	}
	if len(resp.Machines) == 0 {
		return nil, fmt.Errorf("no machine found with ID %q", machineID)
	}
	if len(resp.Machines) > 1 {
		return nil, fmt.Errorf("%d machines found with ID %q", len(resp.Machines), machineID)
	}
	return resp.Machines[0], nil
}

func (cs *consoleServer) getMachineIPMI(machineID string) (*models.V1MachineIPMIResponse, error) {
	resp, err := cs.machineClient.MachineIPMIGet(machineID)
	if err != nil {
		return nil, err
	}
	return resp.Machine, nil
}

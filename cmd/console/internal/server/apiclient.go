package server

import (
	"git.f-i-ts.de/cloud-native/metal/metal-console/metal-api/client/machine"
	"git.f-i-ts.de/cloud-native/metal/metal-console/metal-api/models"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
)

func newMachineClient(url string) *machine.Client {
	transport := httptransport.New(url, "", nil)
	return machine.New(transport, strfmt.Default)
}

func (cs *consoleServer) getMachine(machineID string) (*models.MetalMachine, error) {
	findMachineParams := machine.NewFindMachineParams()
	findMachineParams.ID = machineID
	metalMachine, err := cs.machineClient.FindMachine(findMachineParams)
	if err != nil {
		return nil, err
	}
	return metalMachine.Payload, nil
}

func (cs *consoleServer) getIPMIData(machineID string) (*models.MetalIPMI, error) {
	ipmiDataParams := machine.NewIPMIDataParams()
	ipmiDataParams.ID = machineID
	ipmiData, err := cs.machineClient.IPMIData(ipmiDataParams)
	if err != nil {
		return nil, err
	}
	return ipmiData.Payload, nil
}

package server

import (
	"net/url"

	"git.f-i-ts.de/cloud-native/metal/metal-console/metal-api/client/machine"
	"git.f-i-ts.de/cloud-native/metal/metal-console/metal-api/models"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
)

func newMachineClient(metalURL string) (*machine.Client, error) {
	u, err := url.Parse(metalURL)
	if err != nil {
		return nil, err
	}
	transport := httptransport.New(u.Host, u.Path, nil)
	return machine.New(transport, strfmt.Default), nil
}

func (cs *consoleServer) getMachine(machineID string) (*models.V1MachineResponse, error) {
	findMachineParams := machine.NewFindMachineParams()
	findMachineParams.ID = machineID
	metalMachine, err := cs.machineClient.FindMachine(findMachineParams, cs.Auth)
	if err != nil {
		return nil, err
	}
	return metalMachine.Payload, nil
}

func (cs *consoleServer) getIPMIData(machineID string) (*models.V1MachineIPMI, error) {
	ipmiDataParams := machine.NewIPMIDataParams()
	ipmiDataParams.ID = machineID
	ipmiData, err := cs.machineClient.IPMIData(ipmiDataParams, cs.Auth)
	if err != nil {
		return nil, err
	}
	return ipmiData.Payload, nil
}

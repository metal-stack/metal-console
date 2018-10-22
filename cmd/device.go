package cmd

import (
	"git.f-i-ts.de/cloud-native/maas/metal-console/metal-api/client/device"
	"git.f-i-ts.de/cloud-native/maas/metal-console/metal-api/models"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
)

var (
	transport *httptransport.Runtime
)

func getDevice(url, d string) (*models.MetalDevice, error) {
	transport = httptransport.New(url, "", nil)
	client := device.New(transport, strfmt.Default)

	findDeviceParams := device.NewFindDeviceParams()
	findDeviceParams.ID = d
	metalDevice, err := client.FindDevice(findDeviceParams)
	if err != nil {
		return nil, err
	}
	return metalDevice.Payload, nil
}

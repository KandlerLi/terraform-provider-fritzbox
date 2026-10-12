package tr64

import (
	"context"
	"strconv"
)

const deviceInfoService = "urn:dslforum-org:service:DeviceInfo:1"

type DeviceInfo struct {
	ManufacturerName string
	ModelName        string
	SoftwareVersion  string
	HardwareVersion  string
	UptimeSeconds    int
}

func (c *Client) GetDeviceInfo(ctx context.Context) (*DeviceInfo, error) {
	out, err := c.Call(ctx, deviceInfoService, "GetInfo")
	if err != nil {
		return nil, err
	}
	uptime, _ := strconv.Atoi(out["NewUpTime"])
	return &DeviceInfo{
		ManufacturerName: out["NewManufacturerName"],
		ModelName:        out["NewModelName"],
		SoftwareVersion:  out["NewSoftwareVersion"],
		HardwareVersion:  out["NewHardwareVersion"],
		UptimeSeconds:    uptime,
	}, nil
}

func (c *Client) ExternalIPv4(ctx context.Context, wan string) (string, error) {
	out, err := c.Call(ctx, wan, "GetExternalIPAddress")
	if err != nil {
		return "", err
	}
	return out["NewExternalIPAddress"], nil
}

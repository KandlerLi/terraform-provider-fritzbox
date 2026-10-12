package tr64

import (
	"context"
	"errors"
)

const hostsService = "urn:dslforum-org:service:Hosts:1"

// Host is a device the box knows, active or not.
type Host struct {
	MACAddress string
	IPAddress  string
	HostName   string
	Active     bool
}

// GetHost returns nil, nil if the box does not know the MAC address.
func (c *Client) GetHost(ctx context.Context, mac string) (*Host, error) {
	out, err := c.Call(ctx, hostsService, "GetSpecificHostEntry", Arg{"NewMACAddress", mac})
	var fault *Fault
	if errors.As(err, &fault) && fault.Code == 714 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Host{
		MACAddress: mac,
		IPAddress:  out["NewIPAddress"],
		HostName:   out["NewHostName"],
		Active:     out["NewActive"] == "1",
	}, nil
}

func (c *Client) SetHostName(ctx context.Context, mac, name string) error {
	_, err := c.Call(ctx, hostsService, "X_AVM-DE_SetHostNameByMACAddress",
		Arg{"NewMACAddress", mac},
		Arg{"NewHostName", name},
	)
	return err
}

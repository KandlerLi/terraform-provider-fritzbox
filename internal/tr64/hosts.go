package tr64

import (
	"context"
	"errors"
	"strconv"
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

// ListHosts returns every device the box knows, active or not.
func (c *Client) ListHosts(ctx context.Context) ([]HostEntry, error) {
	var hosts []HostEntry
	// Same end-of-list convention as port mappings: iterate until 713/714.
	for i := 0; ; i++ {
		out, err := c.Call(ctx, hostsService, "GetGenericHostEntry", Arg{"NewIndex", strconv.Itoa(i)})
		var fault *Fault
		if errors.As(err, &fault) && (fault.Code == 713 || fault.Code == 714) {
			return hosts, nil
		}
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, HostEntry{
			Host: Host{
				MACAddress: out["NewMACAddress"],
				IPAddress:  out["NewIPAddress"],
				HostName:   out["NewHostName"],
				Active:     out["NewActive"] == "1",
			},
			InterfaceType: out["NewInterfaceType"],
			AddressSource: out["NewAddressSource"],
		})
	}
}

type HostEntry struct {
	Host
	InterfaceType string // Ethernet, 802.11, ...
	AddressSource string // DHCP or Static
}

const hostFilterService = "urn:dslforum-org:service:X_AVM-DE_HostFilter:1"

func (c *Client) WANAccessBlocked(ctx context.Context, ipv4 string) (bool, error) {
	out, err := c.Call(ctx, hostFilterService, "GetWANAccessByIP", Arg{"NewIPv4Address", ipv4})
	if err != nil {
		return false, err
	}
	return out["NewDisallow"] == "1", nil
}

func (c *Client) SetWANAccessBlocked(ctx context.Context, ipv4 string, blocked bool) error {
	_, err := c.Call(ctx, hostFilterService, "DisallowWANAccessByIP",
		Arg{"NewIPv4Address", ipv4},
		Arg{"NewDisallow", boolArg(blocked)},
	)
	return err
}

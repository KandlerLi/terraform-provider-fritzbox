package tr64

import (
	"context"
	"errors"
	"fmt"
	"strconv"
)

// WAN connection services that can carry port mappings: PPP on DSL boxes, IP on cable/fiber.
var wanServices = []string{
	"urn:dslforum-org:service:WANPPPConnection:1",
	"urn:dslforum-org:service:WANIPConnection:1",
}

type PortMapping struct {
	Protocol       string // TCP or UDP
	ExternalPort   int
	InternalClient string
	InternalPort   int
	Enabled        bool
	Description    string
}

// ActiveWANService returns the WAN connection service that answers port mapping calls; inactive ones fail with 402.
func (c *Client) ActiveWANService(ctx context.Context) (string, error) {
	var errs []error
	for _, svc := range wanServices {
		_, err := c.Call(ctx, svc, "GetPortMappingNumberOfEntries")
		if err == nil {
			return svc, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", svc, err))
	}
	return "", fmt.Errorf("no WAN connection service accepts port mapping calls: %w", errors.Join(errs...))
}

// GetPortMapping returns nil, nil if no mapping exists for protocol/externalPort.
func (c *Client) GetPortMapping(ctx context.Context, wan, protocol string, externalPort int) (*PortMapping, error) {
	out, err := c.Call(ctx, wan, "GetSpecificPortMappingEntry",
		Arg{"NewRemoteHost", ""},
		Arg{"NewExternalPort", strconv.Itoa(externalPort)},
		Arg{"NewProtocol", protocol},
	)
	var fault *Fault
	if errors.As(err, &fault) && fault.Code == 714 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	internalPort, err := strconv.Atoi(out["NewInternalPort"])
	if err != nil {
		return nil, fmt.Errorf("NewInternalPort: %w", err)
	}
	return &PortMapping{
		Protocol:       protocol,
		ExternalPort:   externalPort,
		InternalClient: out["NewInternalClient"],
		InternalPort:   internalPort,
		Enabled:        out["NewEnabled"] == "1",
		Description:    out["NewPortMappingDescription"],
	}, nil
}

// SetPortMapping creates the mapping, or overwrites the one with the same protocol/externalPort.
func (c *Client) SetPortMapping(ctx context.Context, wan string, m PortMapping) error {
	enabled := "0"
	if m.Enabled {
		enabled = "1"
	}
	_, err := c.Call(ctx, wan, "AddPortMapping",
		Arg{"NewRemoteHost", ""},
		Arg{"NewExternalPort", strconv.Itoa(m.ExternalPort)},
		Arg{"NewProtocol", m.Protocol},
		Arg{"NewInternalPort", strconv.Itoa(m.InternalPort)},
		Arg{"NewInternalClient", m.InternalClient},
		Arg{"NewEnabled", enabled},
		Arg{"NewPortMappingDescription", m.Description},
		Arg{"NewLeaseDuration", "0"},
	)
	return err
}

// DeletePortMapping treats an already missing mapping as success.
func (c *Client) DeletePortMapping(ctx context.Context, wan, protocol string, externalPort int) error {
	_, err := c.Call(ctx, wan, "DeletePortMapping",
		Arg{"NewRemoteHost", ""},
		Arg{"NewExternalPort", strconv.Itoa(externalPort)},
		Arg{"NewProtocol", protocol},
	)
	var fault *Fault
	if errors.As(err, &fault) && fault.Code == 714 {
		return nil
	}
	return err
}

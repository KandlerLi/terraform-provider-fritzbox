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
	LeaseDuration  int // seconds; 0 is permanent, anything else was typically opened by a device via UPnP
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
	return parsePortMapping(protocol, externalPort, out)
}

// ListPortMappings returns every mapping on the WAN connection, including ones not created by Terraform.
func (c *Client) ListPortMappings(ctx context.Context, wan string) ([]PortMapping, error) {
	var mappings []PortMapping
	// Iterate until 713/714 rather than trusting GetPortMappingNumberOfEntries, which the spec lets lag behind.
	for i := 0; ; i++ {
		out, err := c.Call(ctx, wan, "GetGenericPortMappingEntry", Arg{"NewPortMappingIndex", strconv.Itoa(i)})
		var fault *Fault
		if errors.As(err, &fault) && (fault.Code == 713 || fault.Code == 714) {
			return mappings, nil
		}
		if err != nil {
			return nil, err
		}
		externalPort, err := strconv.Atoi(out["NewExternalPort"])
		if err != nil {
			return nil, fmt.Errorf("entry %d NewExternalPort: %w", i, err)
		}
		m, err := parsePortMapping(out["NewProtocol"], externalPort, out)
		if err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		mappings = append(mappings, *m)
	}
}

func parsePortMapping(protocol string, externalPort int, out map[string]string) (*PortMapping, error) {
	internalPort, err := strconv.Atoi(out["NewInternalPort"])
	if err != nil {
		return nil, fmt.Errorf("NewInternalPort: %w", err)
	}
	// Informational only, so a missing value must not break reading the mapping.
	lease, _ := strconv.Atoi(out["NewLeaseDuration"])
	return &PortMapping{
		Protocol:       protocol,
		ExternalPort:   externalPort,
		InternalClient: out["NewInternalClient"],
		InternalPort:   internalPort,
		Enabled:        out["NewEnabled"] == "1",
		Description:    out["NewPortMappingDescription"],
		LeaseDuration:  lease,
	}, nil
}

// SetPortMapping creates the mapping, or overwrites the one with the same protocol/externalPort.
func (c *Client) SetPortMapping(ctx context.Context, wan string, m PortMapping) error {
	_, err := c.Call(ctx, wan, "AddPortMapping",
		Arg{"NewRemoteHost", ""},
		Arg{"NewExternalPort", strconv.Itoa(m.ExternalPort)},
		Arg{"NewProtocol", m.Protocol},
		Arg{"NewInternalPort", strconv.Itoa(m.InternalPort)},
		Arg{"NewInternalClient", m.InternalClient},
		Arg{"NewEnabled", boolArg(m.Enabled)},
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

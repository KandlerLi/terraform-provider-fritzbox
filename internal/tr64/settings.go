package tr64

import (
	"context"
	"strconv"
)

const (
	remoteAccessService = "urn:dslforum-org:service:X_AVM-DE_RemoteAccess:1"
	storageService      = "urn:dslforum-org:service:X_AVM-DE_Storage:1"
)

// RemoteAccess is the FRITZ!Box UI's reachability from the internet.
type RemoteAccess struct {
	Enabled bool
	Port    int
}

func (c *Client) GetRemoteAccess(ctx context.Context) (*RemoteAccess, error) {
	out, err := c.Call(ctx, remoteAccessService, "GetInfo")
	if err != nil {
		return nil, err
	}
	port, _ := strconv.Atoi(out["NewPort"])
	return &RemoteAccess{Enabled: out["NewEnabled"] == "1", Port: port}, nil
}

// SetRemoteAccessEnabled uses SetEnable, which unlike SetConfig needs no remote access password.
func (c *Client) SetRemoteAccessEnabled(ctx context.Context, enabled bool) error {
	_, err := c.Call(ctx, remoteAccessService, "SetEnable", Arg{"NewEnabled", boolArg(enabled)})
	return err
}

// Storage is the FRITZ!Box's own NAS sharing (USB storage / internal memory).
type Storage struct {
	FTPEnabled    bool
	FTPWANEnabled bool
	FTPWANSSLOnly bool
	SMBEnabled    bool
}

func (c *Client) GetStorage(ctx context.Context) (*Storage, error) {
	out, err := c.Call(ctx, storageService, "GetInfo")
	if err != nil {
		return nil, err
	}
	return &Storage{
		FTPEnabled:    out["NewFTPEnable"] == "1",
		FTPWANEnabled: out["NewFTPWANEnable"] == "1",
		FTPWANSSLOnly: out["NewFTPWANSSLOnly"] == "1",
		SMBEnabled:    out["NewSMBEnable"] == "1",
	}, nil
}

func (c *Client) SetStorage(ctx context.Context, s Storage) error {
	if _, err := c.Call(ctx, storageService, "SetFTPServer", Arg{"NewFTPEnable", boolArg(s.FTPEnabled)}); err != nil {
		return err
	}
	if _, err := c.Call(ctx, storageService, "SetFTPServerWAN",
		Arg{"NewFTPWANEnable", boolArg(s.FTPWANEnabled)},
		Arg{"NewFTPWANSSLOnly", boolArg(s.FTPWANSSLOnly)},
	); err != nil {
		return err
	}
	_, err := c.Call(ctx, storageService, "SetSMBServer", Arg{"NewSMBEnable", boolArg(s.SMBEnabled)})
	return err
}

func boolArg(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// DynamicDNS is the box's DynDNS client. The credentials are write-only: GetDDNSInfo returns
// the username, but it is deliberately not exposed, and the password is never returned.
type DynamicDNS struct {
	Enabled      bool
	ProviderName string
	UpdateURL    string
	Domain       string
	Mode         string // ddns_v4, ddns_v6, ddns_both or ddns_together
	ServerIPv4   string
	ServerIPv6   string
	StatusIPv4   string
	StatusIPv6   string
}

func (c *Client) GetDynamicDNS(ctx context.Context) (*DynamicDNS, error) {
	out, err := c.Call(ctx, remoteAccessService, "GetDDNSInfo")
	if err != nil {
		return nil, err
	}
	return &DynamicDNS{
		Enabled:      out["NewEnabled"] == "1",
		ProviderName: out["NewProviderName"],
		UpdateURL:    out["NewUpdateURL"],
		Domain:       out["NewDomain"],
		Mode:         out["NewMode"],
		ServerIPv4:   out["NewServerIPv4"],
		ServerIPv6:   out["NewServerIPv6"],
		StatusIPv4:   out["NewStatusIPv4"],
		StatusIPv6:   out["NewStatusIPv6"],
	}, nil
}

// SetDynamicDNS needs the credentials on every call: SetDDNSConfig has no way to keep the stored ones.
func (c *Client) SetDynamicDNS(ctx context.Context, d DynamicDNS, username, password string) error {
	_, err := c.Call(ctx, remoteAccessService, "SetDDNSConfig",
		Arg{"NewEnabled", boolArg(d.Enabled)},
		Arg{"NewProviderName", d.ProviderName},
		Arg{"NewUpdateURL", d.UpdateURL},
		Arg{"NewDomain", d.Domain},
		Arg{"NewUsername", username},
		Arg{"NewMode", d.Mode},
		Arg{"NewServerIPv4", d.ServerIPv4},
		Arg{"NewServerIPv6", d.ServerIPv6},
		Arg{"NewPassword", password},
	)
	return err
}

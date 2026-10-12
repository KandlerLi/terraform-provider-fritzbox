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

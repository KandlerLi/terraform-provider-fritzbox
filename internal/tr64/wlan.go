package tr64

import (
	"context"
	"fmt"
)

func wlanService(index int) string {
	return fmt.Sprintf("urn:dslforum-org:service:WLANConfiguration:%d", index)
}

// WLAN is one radio/SSID. The passphrase is write-only: GetSecurityKeys would return it, so it is never read.
type WLAN struct {
	Enabled       bool
	SSID          string
	SSIDBroadcast bool
	Security      string // BeaconType: None, 11i (WPA2), 11iandWPA3, WPA3, ...
	FrequencyBand string // 2400, 5000, 6000
	APType        string // normal or guest
}

func (c *Client) GetWLAN(ctx context.Context, index int) (*WLAN, error) {
	svc := wlanService(index)
	info, err := c.Call(ctx, svc, "GetInfo")
	if err != nil {
		return nil, err
	}
	adv, err := c.Call(ctx, svc, "GetBeaconAdvertisement")
	if err != nil {
		return nil, err
	}
	ext, err := c.Call(ctx, svc, "X_AVM-DE_GetWLANExtInfo")
	if err != nil {
		return nil, err
	}
	return &WLAN{
		Enabled:       info["NewEnable"] == "1",
		SSID:          info["NewSSID"],
		SSIDBroadcast: adv["NewBeaconAdvertisementEnabled"] == "1",
		Security:      info["NewBeaconType"],
		FrequencyBand: info["NewX_AVM-DE_FrequencyBand"],
		APType:        ext["NewX_AVM-DE_APType"],
	}, nil
}

// SetWLAN only calls the setters whose value differs from current, so an unchanged radio is never touched.
func (c *Client) SetWLAN(ctx context.Context, index int, current *WLAN, want WLAN) error {
	svc := wlanService(index)
	if want.SSID != current.SSID {
		if _, err := c.Call(ctx, svc, "SetSSID", Arg{"NewSSID", want.SSID}); err != nil {
			return err
		}
	}
	if want.Security != current.Security {
		if _, err := c.Call(ctx, svc, "SetBeaconType", Arg{"NewBeaconType", want.Security}); err != nil {
			return err
		}
	}
	if want.SSIDBroadcast != current.SSIDBroadcast {
		if _, err := c.Call(ctx, svc, "SetBeaconAdvertisement", Arg{"NewBeaconAdvertisementEnabled", boolArg(want.SSIDBroadcast)}); err != nil {
			return err
		}
	}
	if want.Enabled != current.Enabled {
		if _, err := c.Call(ctx, svc, "SetEnable", Arg{"NewEnable", boolArg(want.Enabled)}); err != nil {
			return err
		}
	}
	return nil
}

// SetWLANPassphrase sets the WPA passphrase; the WEP keys and raw PSK are left empty, as the UI does.
func (c *Client) SetWLANPassphrase(ctx context.Context, index int, passphrase string) error {
	_, err := c.Call(ctx, wlanService(index), "SetSecurityKeys",
		Arg{"NewWEPKey0", ""},
		Arg{"NewWEPKey1", ""},
		Arg{"NewWEPKey2", ""},
		Arg{"NewWEPKey3", ""},
		Arg{"NewPreSharedKey", ""},
		Arg{"NewKeyPassphrase", passphrase},
	)
	return err
}

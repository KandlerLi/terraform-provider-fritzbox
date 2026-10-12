package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestWLAN(t *testing.T) {
	box, providerConfig := newFakeFritzBox(t)
	config := func(ssid, passphrase string) string {
		return providerConfig + fmt.Sprintf(`
resource "fritzbox_wlan" "guest" {
  index    = 3
  enabled  = true
  ssid     = %q
  security = "11iandWPA3"
  %s
}`, ssid, passphrase)
	}
	calls := func(action string, want int) resource.TestCheckFunc {
		return func(*terraform.State) error {
			if got := box.callCount(action); got != want {
				return fmt.Errorf("%s called %d times, want %d", action, got, want)
			}
			return nil
		}
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("Gast", `passphrase_wo = "pw1"
  passphrase_wo_version = 1`),
				Check: resource.ComposeAggregateTestCheckFunc(
					onBoxSetting(box, wlan3Service, "NewEnable", "1"),
					onBoxSetting(box, wlan3Service, "NewSSID", "Gast"),
					onBoxSetting(box, wlan3Service, "NewBeaconType", "11iandWPA3"),
					onBoxSetting(box, wlan3Service, "NewKeyPassphrase", "pw1"),
					resource.TestCheckResourceAttr("fritzbox_wlan.guest", "ap_type", "guest"),
					resource.TestCheckNoResourceAttr("fritzbox_wlan.guest", "passphrase_wo"),
				),
			},
			{
				// Same version: the passphrase stays in config but must not be re-sent.
				Config: config("Gast2", `passphrase_wo = "pw1"
  passphrase_wo_version = 1`),
				Check: resource.ComposeAggregateTestCheckFunc(
					onBoxSetting(box, wlan3Service, "NewSSID", "Gast2"),
					calls("SetSecurityKeys", 1),
				),
			},
			{
				Config: config("Gast2", `passphrase_wo = "pw2"
  passphrase_wo_version = 2`),
				Check: resource.ComposeAggregateTestCheckFunc(
					onBoxSetting(box, wlan3Service, "NewKeyPassphrase", "pw2"),
					calls("SetSecurityKeys", 2),
				),
			},
			{
				Config:      config("Gast2", `passphrase_wo_version = 3`),
				ExpectError: regexp.MustCompile(`passphrase_wo is not set`),
			},
			{
				ResourceName:            "fritzbox_wlan.guest",
				ImportState:             true,
				ImportStateId:           "3",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"passphrase_wo_version"},
				// The key must never be read from the box, or it would end up in state.
				Check: calls("GetSecurityKeys", 0),
			},
		},
		CheckDestroy: func(s *terraform.State) error {
			if box.callCount("GetSecurityKeys") != 0 {
				return fmt.Errorf("GetSecurityKeys was called")
			}
			return onBoxSetting(box, wlan3Service, "NewEnable", "1")(s)
		},
	})
}

func TestWANAccessBlock(t *testing.T) {
	box, providerConfig := newFakeFritzBox(t)
	isBlocked := func(want bool) resource.TestCheckFunc {
		return func(*terraform.State) error {
			box.mu.Lock()
			defer box.mu.Unlock()
			if got := box.blocked["192.168.178.50"]; got != want {
				return fmt.Errorf("blocked = %t, want %t", got, want)
			}
			return nil
		}
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `resource "fritzbox_wan_access_block" "tv" { ip_address = "192.168.178.50" }`,
				Check:  isBlocked(true),
			},
			{
				ResourceName:      "fritzbox_wan_access_block.tv",
				ImportState:       true,
				ImportStateId:     "192.168.178.50",
				ImportStateVerify: true,
			},
		},
		// Unlike settings, destroying the block lifts it.
		CheckDestroy: isBlocked(false),
	})
}

func TestInfoDataSources(t *testing.T) {
	_, providerConfig := newFakeFritzBox(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{{
			Config: providerConfig + `
data "fritzbox_device_info" "this" {}
data "fritzbox_hosts" "all" {}`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.fritzbox_device_info.this", "model_name", "FRITZ!Box 7530 AX"),
				resource.TestCheckResourceAttr("data.fritzbox_device_info.this", "software_version", "256.08.25"),
				resource.TestCheckResourceAttr("data.fritzbox_device_info.this", "uptime_seconds", "3600"),
				resource.TestCheckResourceAttr("data.fritzbox_device_info.this", "external_ipv4", "203.0.113.7"),
				resource.TestCheckNoResourceAttr("data.fritzbox_device_info.this", "serial_number"),
				resource.TestCheckResourceAttr("data.fritzbox_hosts.all", "hosts.#", "1"),
				resource.TestCheckResourceAttr("data.fritzbox_hosts.all", "hosts.0.mac_address", "AA:BB:CC:DD:EE:01"),
				resource.TestCheckResourceAttr("data.fritzbox_hosts.all", "hosts.0.ip_address", "192.168.178.100"),
				resource.TestCheckResourceAttr("data.fritzbox_hosts.all", "hosts.0.interface_type", "Ethernet"),
			),
		}},
	})
}

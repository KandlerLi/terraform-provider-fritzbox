package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestPortForward(t *testing.T) {
	box, providerConfig := newFakeFritzBox(t)
	box.mappings["TCP/25"] = map[string]string{"NewInternalPort": "25", "NewInternalClient": "192.168.178.100", "NewEnabled": "1", "NewPortMappingDescription": "E-Mail"}

	onBox := func(key, field, want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			box.mu.Lock()
			defer box.mu.Unlock()
			if got := box.mappings[key][field]; got != want {
				return fmt.Errorf("box %s %s = %q, want %q", key, field, got, want)
			}
			return nil
		}
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "fritzbox_port_forward" "https" {
  protocol        = "TCP"
  external_port   = 443
  internal_client = "192.168.178.100"
  internal_port   = 443
  description     = "HTTPS"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fritzbox_port_forward.https", "id", "TCP/443"),
					resource.TestCheckResourceAttr("fritzbox_port_forward.https", "enabled", "true"),
					onBox("TCP/443", "NewInternalClient", "192.168.178.100"),
					onBox("TCP/443", "NewEnabled", "1"),
				),
			},
			{
				Config: providerConfig + `
resource "fritzbox_port_forward" "https" {
  protocol        = "TCP"
  external_port   = 443
  internal_client = "192.168.178.100"
  internal_port   = 8443
  enabled         = false
  description     = "HTTPS"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					onBox("TCP/443", "NewInternalPort", "8443"),
					onBox("TCP/443", "NewEnabled", "0"),
				),
			},
			{
				ResourceName:      "fritzbox_port_forward.https",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// TCP/25 exists on the box but not in state: creating it must fail instead of overwriting.
				Config: providerConfig + `
resource "fritzbox_port_forward" "smtp" {
  protocol        = "TCP"
  external_port   = 25
  internal_client = "192.168.178.200"
  internal_port   = 25
}`,
				ExpectError: regexp.MustCompile(`Port forward already exists`),
			},
		},
		CheckDestroy: func(*terraform.State) error {
			box.mu.Lock()
			defer box.mu.Unlock()
			if _, ok := box.mappings["TCP/443"]; ok {
				return fmt.Errorf("TCP/443 still on the box after destroy")
			}
			if _, ok := box.mappings["TCP/25"]; !ok {
				return fmt.Errorf("unmanaged TCP/25 was removed")
			}
			return nil
		},
	})
}

func TestPortForwardsDataSource(t *testing.T) {
	box, providerConfig := newFakeFritzBox(t)
	box.mappings["TCP/25"] = map[string]string{"NewInternalPort": "25", "NewInternalClient": "192.168.178.100", "NewEnabled": "1", "NewPortMappingDescription": "E-Mail", "NewLeaseDuration": "0"}
	box.mappings["UDP/3074"] = map[string]string{"NewInternalPort": "3074", "NewInternalClient": "192.168.178.50", "NewEnabled": "1", "NewPortMappingDescription": "console", "NewLeaseDuration": "3600"}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{{
			Config: providerConfig + `data "fritzbox_port_forwards" "all" {}`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.fritzbox_port_forwards.all", "port_forwards.#", "2"),
				resource.TestCheckResourceAttr("data.fritzbox_port_forwards.all", "port_forwards.0.id", "TCP/25"),
				resource.TestCheckResourceAttr("data.fritzbox_port_forwards.all", "port_forwards.1.id", "UDP/3074"),
				resource.TestCheckResourceAttr("data.fritzbox_port_forwards.all", "port_forwards.1.internal_client", "192.168.178.50"),
				resource.TestCheckResourceAttr("data.fritzbox_port_forwards.all", "port_forwards.1.lease_duration", "3600"),
			),
		}},
	})
}

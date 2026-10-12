package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestDynamicDNS(t *testing.T) {
	box, providerConfig := newFakeFritzBox(t)
	onBox := func(field, want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			box.mu.Lock()
			defer box.mu.Unlock()
			if got := box.ddns[field]; got != want {
				return fmt.Errorf("box DDNS %s = %q, want %q", field, got, want)
			}
			return nil
		}
	}
	config := func(domain, credentials string) string {
		return providerConfig + fmt.Sprintf(`
resource "fritzbox_dynamic_dns" "this" {
  enabled       = true
  provider_name = "userdefined"
  update_url    = "https://<username>:<pass>@example.com/nic/update?hostname=<domain>&myip=<ipaddr>"
  domain        = %q
  mode          = "ddns_v4"
  %s
}`, domain, credentials)
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("example.com", `username_wo = "user1"
  password_wo = "secret1"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					onBox("NewEnabled", "1"),
					onBox("NewUsername", "user1"),
					onBox("NewPassword", "secret1"),
					resource.TestCheckNoResourceAttr("fritzbox_dynamic_dns.this", "password_wo"),
					resource.TestCheckNoResourceAttr("fritzbox_dynamic_dns.this", "username_wo"),
					resource.TestCheckResourceAttr("fritzbox_dynamic_dns.this", "status_ipv4", "updated"),
				),
			},
			{
				// Without credentials the box would lose its password, so the change is refused.
				Config:      config("www.example.com", ""),
				ExpectError: regexp.MustCompile(`username_wo and password_wo are required`),
			},
			{
				// Rotation: only the version changes, which must still push the new password.
				Config: config("example.com", `username_wo = "user1"
  password_wo = "secret2"
  credentials_wo_version = 2`),
				Check: onBox("NewPassword", "secret2"),
			},
			{
				ResourceName:            "fritzbox_dynamic_dns.this",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"credentials_wo_version"},
			},
		},
		// Destroy only forgets the setting.
		CheckDestroy: onBox("NewEnabled", "1"),
	})
}

package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestHostName(t *testing.T) {
	box, providerConfig := newFakeFritzBox(t)
	config := func(mac, name string) string {
		return providerConfig + fmt.Sprintf(`
resource "fritzbox_host_name" "homeserver" {
  mac_address = %q
  host_name   = %q
}`, mac, name)
	}
	nameOnBox := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			box.mu.Lock()
			defer box.mu.Unlock()
			if got := box.hosts["AA:BB:CC:DD:EE:01"]["NewHostName"]; got != want {
				return fmt.Errorf("box host name = %q, want %q", got, want)
			}
			return nil
		}
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config("aa:bb:cc:dd:ee:01", "homeserver"),
				ExpectError: regexp.MustCompile(`must be upper case`),
			},
			{
				Config:      config("AA:BB:CC:DD:EE:99", "ghost"),
				ExpectError: regexp.MustCompile(`NoSuchEntryInArray`),
			},
			{
				Config: config("AA:BB:CC:DD:EE:01", "homeserver"),
				Check: resource.ComposeAggregateTestCheckFunc(
					nameOnBox("homeserver"),
					resource.TestCheckResourceAttr("fritzbox_host_name.homeserver", "ip_address", "192.168.178.100"),
					resource.TestCheckResourceAttr("fritzbox_host_name.homeserver", "active", "true"),
				),
			},
			{
				Config: config("AA:BB:CC:DD:EE:01", "nas"),
				Check:  nameOnBox("nas"),
			},
			{
				ResourceName:      "fritzbox_host_name.homeserver",
				ImportState:       true,
				ImportStateId:     "AA:BB:CC:DD:EE:01",
				ImportStateVerify: true,
			},
		},
		// Destroy only forgets the name.
		CheckDestroy: nameOnBox("nas"),
	})
}

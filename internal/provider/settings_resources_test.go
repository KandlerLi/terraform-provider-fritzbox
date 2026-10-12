package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func onBoxSetting(box *fakeFritzBox, service, field, want string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if got := box.get(service, field); got != want {
			return fmt.Errorf("box %s %s = %q, want %q", service, field, got, want)
		}
		return nil
	}
}

func TestRemoteAccess(t *testing.T) {
	box, providerConfig := newFakeFritzBox(t)
	config := func(enabled bool) string {
		return providerConfig + fmt.Sprintf(`resource "fritzbox_remote_access" "this" { enabled = %t }`, enabled)
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fritzbox_remote_access.this", "port", "44443"),
					onBoxSetting(box, remoteService, "NewEnabled", "1"),
				),
			},
			{
				Config: config(false),
				Check:  onBoxSetting(box, remoteService, "NewEnabled", "0"),
			},
			{
				ResourceName:      "fritzbox_remote_access.this",
				ImportState:       true,
				ImportStateId:     "anything",
				ImportStateVerify: true,
			},
		},
		// Destroy only forgets the setting.
		CheckDestroy: onBoxSetting(box, remoteService, "NewEnabled", "0"),
	})
}

func TestStorage(t *testing.T) {
	box, providerConfig := newFakeFritzBox(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				// The fake starts with SMB and SSL-only on, both off by default.
				Config: providerConfig + `resource "fritzbox_storage" "this" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fritzbox_storage.this", "ftp_enabled", "true"),
					onBoxSetting(box, storageService, "NewFTPEnable", "1"),
					onBoxSetting(box, storageService, "NewFTPWANSSLOnly", "0"),
					onBoxSetting(box, storageService, "NewSMBEnable", "0"),
				),
			},
			{
				Config: providerConfig + `
resource "fritzbox_storage" "this" {
  ftp_enabled      = false
  ftp_wan_enabled  = false
  ftp_wan_ssl_only = true
  smb_enabled      = false
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					onBoxSetting(box, storageService, "NewFTPEnable", "0"),
					onBoxSetting(box, storageService, "NewFTPWANEnable", "0"),
					onBoxSetting(box, storageService, "NewSMBEnable", "0"),
				),
			},
			{
				ResourceName:      "fritzbox_storage.this",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
		CheckDestroy: onBoxSetting(box, storageService, "NewSMBEnable", "0"),
	})
}

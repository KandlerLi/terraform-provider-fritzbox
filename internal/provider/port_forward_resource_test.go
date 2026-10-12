package provider

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	pppService = "urn:dslforum-org:service:WANPPPConnection:1"
	ipService  = "urn:dslforum-org:service:WANIPConnection:1"
)

// fakeFritzBox mimics a DSL box: port mappings live on WANPPPConnection, WANIPConnection answers 402.
type fakeFritzBox struct {
	mu       sync.Mutex
	mappings map[string]map[string]string // "TCP/443" -> output args
}

func (f *fakeFritzBox) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/tr64desc.xml" {
		fmt.Fprintf(w, `<root><device><deviceList><device><deviceList><device><serviceList>
<service><serviceType>%s</serviceType><controlURL>/upnp/control/wanpppconn1</controlURL></service>
<service><serviceType>%s</serviceType><controlURL>/upnp/control/wanipconnection1</controlURL></service>
</serviceList></device></deviceList></device></deviceList></device></root>`, pppService, ipService)
		return
	}
	service, action, _ := strings.Cut(r.Header.Get("SOAPACTION"), "#")
	if service == ipService {
		fault(w, 402, "Invalid Args")
		return
	}

	var env struct {
		Body struct {
			Action struct {
				Args []struct {
					XMLName xml.Name
					Value   string `xml:",chardata"`
				} `xml:",any"`
			} `xml:",any"`
		}
	}
	if err := xml.NewDecoder(r.Body).Decode(&env); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	args := map[string]string{}
	for _, a := range env.Body.Action.Args {
		args[a.XMLName.Local] = a.Value
	}
	key := args["NewProtocol"] + "/" + args["NewExternalPort"]

	f.mu.Lock()
	defer f.mu.Unlock()
	switch action {
	case "GetPortMappingNumberOfEntries":
		respond(w, action, map[string]string{"NewPortMappingNumberOfEntries": fmt.Sprint(len(f.mappings))})
	case "GetSpecificPortMappingEntry":
		m, ok := f.mappings[key]
		if !ok {
			fault(w, 714, "NoSuchEntryInArray")
			return
		}
		respond(w, action, m)
	case "AddPortMapping":
		f.mappings[key] = map[string]string{
			"NewInternalPort":           args["NewInternalPort"],
			"NewInternalClient":         args["NewInternalClient"],
			"NewEnabled":                args["NewEnabled"],
			"NewPortMappingDescription": args["NewPortMappingDescription"],
			"NewLeaseDuration":          args["NewLeaseDuration"],
		}
		respond(w, action, nil)
	case "DeletePortMapping":
		if _, ok := f.mappings[key]; !ok {
			fault(w, 714, "NoSuchEntryInArray")
			return
		}
		delete(f.mappings, key)
		respond(w, action, nil)
	default:
		fault(w, 401, "Invalid Action")
	}
}

func respond(w http.ResponseWriter, action string, out map[string]string) {
	fmt.Fprintf(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:%sResponse xmlns:u="%s">`, action, pppService)
	for k, v := range out {
		fmt.Fprintf(w, "<%s>", k)
		xml.EscapeText(w, []byte(v))
		fmt.Fprintf(w, "</%s>", k)
	}
	fmt.Fprintf(w, "</u:%sResponse></s:Body></s:Envelope>", action)
}

func fault(w http.ResponseWriter, code int, desc string) {
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><s:Fault><detail><UPnPError><errorCode>%d</errorCode><errorDescription>%s</errorDescription></UPnPError></detail></s:Fault></s:Body></s:Envelope>`, code, desc)
}

func TestPortForward(t *testing.T) {
	box := &fakeFritzBox{mappings: map[string]map[string]string{
		"TCP/25": {"NewInternalPort": "25", "NewInternalClient": "192.168.178.100", "NewEnabled": "1", "NewPortMappingDescription": "E-Mail"},
	}}
	srv := httptest.NewServer(box)
	defer srv.Close()

	providerConfig := fmt.Sprintf(`
provider "fritzbox" {
  url      = %q
  username = "test"
  password = "test"
}
`, srv.URL)
	factories := map[string]func() (tfprotov6.ProviderServer, error){
		"fritzbox": providerserver.NewProtocol6WithError(New("test")()),
	}
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
		ProtoV6ProviderFactories: factories,
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

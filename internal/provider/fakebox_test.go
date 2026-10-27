package provider

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

const (
	pppService     = "urn:dslforum-org:service:WANPPPConnection:1"
	ipService      = "urn:dslforum-org:service:WANIPConnection:1"
	remoteService  = "urn:dslforum-org:service:X_AVM-DE_RemoteAccess:1"
	storageService = "urn:dslforum-org:service:X_AVM-DE_Storage:1"
	hostsService   = "urn:dslforum-org:service:Hosts:1"
	wlan1Service   = "urn:dslforum-org:service:WLANConfiguration:1"
	wlan3Service   = "urn:dslforum-org:service:WLANConfiguration:3"
	filterService  = "urn:dslforum-org:service:X_AVM-DE_HostFilter:1"
	infoService    = "urn:dslforum-org:service:DeviceInfo:1"
	myFritzService = "urn:dslforum-org:service:X_AVM-DE_MyFritz:1"
)

// fakeFritzBox mimics a DSL box: port mappings live on WANPPPConnection, WANIPConnection answers 402.
// Settings services answer GetInfo from settings[service], and every Set* merges its arguments
// into it, which works because the box uses the same argument names for both.
type fakeFritzBox struct {
	mu       sync.Mutex
	mappings map[string]map[string]string // "TCP/443" -> output args
	settings map[string]map[string]string // service type -> GetInfo output args
	ddns     map[string]string            // SetDDNSConfig args, including NewPassword
	hosts    map[string]map[string]string // MAC -> GetSpecificHostEntry output args
	blocked  map[string]bool              // IPv4 -> internet access blocked
	calls    map[string]int               // action -> number of calls
}

func newFakeFritzBox(t *testing.T) (*fakeFritzBox, string) {
	box := &fakeFritzBox{
		mappings: map[string]map[string]string{},
		settings: map[string]map[string]string{
			remoteService:  {"NewEnabled": "0", "NewPort": "44443"},
			storageService: {"NewFTPEnable": "1", "NewFTPWANEnable": "0", "NewFTPWANSSLOnly": "1", "NewSMBEnable": "1"},
			wlan1Service: {"NewEnable": "1", "NewSSID": "FRITZ!Box 7530 AX", "NewBeaconType": "11i", "NewBeaconAdvertisementEnabled": "1",
				"NewX_AVM-DE_FrequencyBand": "2400", "NewX_AVM-DE_APType": "normal"},
			wlan3Service: {"NewEnable": "0", "NewSSID": "FRITZ!Box Gastzugang", "NewBeaconType": "11i", "NewBeaconAdvertisementEnabled": "1",
				"NewX_AVM-DE_FrequencyBand": "2400", "NewX_AVM-DE_APType": "guest"},
			infoService: {"NewManufacturerName": "AVM", "NewModelName": "FRITZ!Box 7530 AX", "NewSoftwareVersion": "256.08.25",
				"NewHardwareVersion": "FRITZ!Box 7530 AX", "NewUpTime": "3600", "NewSerialNumber": "should-not-leak"},
			myFritzService: {"NewEnabled": "1", "NewDeviceRegistered": "1", "NewState": "myfritz_registered",
				"NewEmail": "owner@example.org", "NewNumberOfServices": "1"},
		},
		blocked: map[string]bool{},
		calls:   map[string]int{},
		ddns:    map[string]string{"NewEnabled": "0", "NewMode": "ddns_v4"},
		hosts: map[string]map[string]string{
			"AA:BB:CC:DD:EE:01": {"NewIPAddress": "192.168.178.100", "NewHostName": "PC-192-168-178-100", "NewActive": "1"},
		},
	}
	srv := httptest.NewServer(box)
	t.Cleanup(srv.Close)
	return box, fmt.Sprintf(`
provider "fritzbox" {
  url      = %q
  username = "test"
  password = "test"
}
`, srv.URL)
}

var testProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"fritzbox": providerserver.NewProtocol6WithError(New("test")()),
}

func (f *fakeFritzBox) callCount(action string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[action]
}

func (f *fakeFritzBox) get(service, field string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.settings[service][field]
}

func (f *fakeFritzBox) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/tr64desc.xml" {
		fmt.Fprint(w, `<root><device><deviceList><device><deviceList><device><serviceList>`)
		for _, s := range []string{pppService, ipService, remoteService, storageService, hostsService, wlan1Service, wlan3Service, filterService, infoService, myFritzService} {
			fmt.Fprintf(w, `<service><serviceType>%s</serviceType><controlURL>/upnp/control/%s</controlURL></service>`, s, strings.ToLower(s[strings.LastIndex(s, "service:")+8:]))
		}
		fmt.Fprint(w, `</serviceList></device></deviceList></device></deviceList></device></root>`)
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

	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[action]++
	switch action {
	case "GetDDNSInfo":
		out := map[string]string{"NewStatusIPv4": "updated"}
		for k, v := range f.ddns {
			if k != "NewPassword" {
				out[k] = v
			}
		}
		respond(w, service, action, out)
		return
	case "SetDDNSConfig":
		f.ddns = args
		respond(w, service, action, nil)
		return
	case "GetWANAccessByIP":
		respond(w, service, action, map[string]string{"NewDisallow": boolArgFake(f.blocked[args["NewIPv4Address"]])})
		return
	case "DisallowWANAccessByIP":
		f.blocked[args["NewIPv4Address"]] = args["NewDisallow"] == "1"
		respond(w, service, action, nil)
		return
	case "GetExternalIPAddress":
		respond(w, service, action, map[string]string{"NewExternalIPAddress": "203.0.113.7"})
		return
	case "GetGenericHostEntry":
		macs := make([]string, 0, len(f.hosts))
		for m := range f.hosts {
			macs = append(macs, m)
		}
		sort.Strings(macs)
		var i int
		fmt.Sscan(args["NewIndex"], &i)
		if i >= len(macs) {
			fault(w, 713, "SpecifiedArrayIndexInvalid")
			return
		}
		out := map[string]string{"NewMACAddress": macs[i], "NewInterfaceType": "Ethernet", "NewAddressSource": "DHCP"}
		for k, v := range f.hosts[macs[i]] {
			out[k] = v
		}
		respond(w, service, action, out)
		return
	case "GetSpecificHostEntry", "X_AVM-DE_SetHostNameByMACAddress":
		h, ok := f.hosts[args["NewMACAddress"]]
		if !ok {
			fault(w, 714, "NoSuchEntryInArray")
			return
		}
		if action == "GetSpecificHostEntry" {
			respond(w, service, action, h)
		} else {
			h["NewHostName"] = args["NewHostName"]
			respond(w, service, action, nil)
		}
		return
	}
	if settings, ok := f.settings[service]; ok {
		switch {
		case strings.HasPrefix(action, "Get") || strings.HasPrefix(action, "X_AVM-DE_Get"):
			respond(w, service, action, settings)
		case strings.HasPrefix(action, "Set"):
			for k, v := range args {
				settings[k] = v
			}
			respond(w, service, action, nil)
		default:
			fault(w, 401, "Invalid Action")
		}
		return
	}

	key := args["NewProtocol"] + "/" + args["NewExternalPort"]
	switch action {
	case "GetPortMappingNumberOfEntries":
		respond(w, service, action, map[string]string{"NewPortMappingNumberOfEntries": fmt.Sprint(len(f.mappings))})
	case "GetGenericPortMappingEntry":
		keys := make([]string, 0, len(f.mappings))
		for k := range f.mappings {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var i int
		fmt.Sscan(args["NewPortMappingIndex"], &i)
		if i >= len(keys) {
			fault(w, 713, "SpecifiedArrayIndexInvalid")
			return
		}
		protocol, port, _ := strings.Cut(keys[i], "/")
		out := map[string]string{"NewProtocol": protocol, "NewExternalPort": port}
		for k, v := range f.mappings[keys[i]] {
			out[k] = v
		}
		respond(w, service, action, out)
	case "GetSpecificPortMappingEntry":
		m, ok := f.mappings[key]
		if !ok {
			fault(w, 714, "NoSuchEntryInArray")
			return
		}
		respond(w, service, action, m)
	case "AddPortMapping":
		f.mappings[key] = map[string]string{
			"NewInternalPort":           args["NewInternalPort"],
			"NewInternalClient":         args["NewInternalClient"],
			"NewEnabled":                args["NewEnabled"],
			"NewPortMappingDescription": args["NewPortMappingDescription"],
			"NewLeaseDuration":          args["NewLeaseDuration"],
		}
		respond(w, service, action, nil)
	case "DeletePortMapping":
		if _, ok := f.mappings[key]; !ok {
			fault(w, 714, "NoSuchEntryInArray")
			return
		}
		delete(f.mappings, key)
		respond(w, service, action, nil)
	default:
		fault(w, 401, "Invalid Action")
	}
}

func respond(w http.ResponseWriter, service, action string, out map[string]string) {
	fmt.Fprintf(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:%sResponse xmlns:u="%s">`, action, service)
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

func boolArgFake(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

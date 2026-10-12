// Package tr64 is a minimal TR-064 (SOAP over HTTP digest auth) client for FRITZ!Box routers.
package tr64

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

type Client struct {
	BaseURL  string // e.g. http://fritz.box:49000
	User     string
	Password string
	HTTP     *http.Client

	services map[string]string // serviceType -> controlURL
}

// Fault is a UPnP error returned by the device, e.g. 714 NoSuchEntryInArray.
type Fault struct {
	Code        int
	Description string
}

func (f *Fault) Error() string { return fmt.Sprintf("UPnP error %d: %s", f.Code, f.Description) }

// Discover reads tr64desc.xml so callers can check which services this model/firmware offers.
func (c *Client) Discover(ctx context.Context) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/tr64desc.xml", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tr64desc.xml: HTTP %d", resp.StatusCode)
	}

	var desc struct {
		Device device `xml:"device"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&desc); err != nil {
		return nil, fmt.Errorf("tr64desc.xml: %w", err)
	}
	c.services = map[string]string{}
	desc.Device.collect(c.services)
	return c.services, nil
}

type device struct {
	Services []struct {
		Type       string `xml:"serviceType"`
		ControlURL string `xml:"controlURL"`
	} `xml:"serviceList>service"`
	Devices []device `xml:"deviceList>device"`
}

func (d device) collect(into map[string]string) {
	for _, s := range d.Services {
		into[s.Type] = s.ControlURL
	}
	for _, sub := range d.Devices {
		sub.collect(into)
	}
}

// Arg is one SOAP input argument; order matters, as UPnP requires arguments in SCPD order.
type Arg struct{ Name, Value string }

// Call invokes a SOAP action and returns its output arguments by name.
func (c *Client) Call(ctx context.Context, serviceType, action string, args ...Arg) (map[string]string, error) {
	if c.services == nil {
		if _, err := c.Discover(ctx); err != nil {
			return nil, err
		}
	}
	controlURL, ok := c.services[serviceType]
	if !ok {
		return nil, fmt.Errorf("service %s not offered by this device", serviceType)
	}

	var body bytes.Buffer
	fmt.Fprintf(&body, `<?xml version="1.0" encoding="utf-8"?><s:Envelope s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/" xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:%s xmlns:u="%s">`, action, serviceType)
	for _, a := range args {
		fmt.Fprintf(&body, "<%s>", a.Name)
		xml.EscapeText(&body, []byte(a.Value))
		fmt.Fprintf(&body, "</%s>", a.Name)
	}
	fmt.Fprintf(&body, "</u:%s></s:Body></s:Envelope>", action)

	newReq := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+controlURL, bytes.NewReader(body.Bytes()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
		req.Header.Set("SOAPACTION", serviceType+"#"+action)
		return req, nil
	}

	req, err := newReq()
	if err != nil {
		return nil, err
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		challenge := resp.Header.Get("WWW-Authenticate")
		resp.Body.Close()
		if req, err = newReq(); err != nil {
			return nil, err
		}
		auth, err := c.digest(challenge, http.MethodPost, controlURL)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", auth)
		if resp, err = c.http().Do(req); err != nil {
			return nil, err
		}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return parseResponse(raw)
	case http.StatusInternalServerError:
		var env struct {
			Code int    `xml:"Body>Fault>detail>UPnPError>errorCode"`
			Desc string `xml:"Body>Fault>detail>UPnPError>errorDescription"`
		}
		if xml.Unmarshal(raw, &env) == nil && env.Code != 0 {
			return nil, &Fault{Code: env.Code, Description: env.Desc}
		}
	}
	return nil, fmt.Errorf("%s#%s: HTTP %d", serviceType, action, resp.StatusCode)
}

func parseResponse(raw []byte) (map[string]string, error) {
	var env struct {
		Body struct {
			Response []byte `xml:",innerxml"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	var resp struct {
		Fields []struct {
			XMLName xml.Name
			Value   string `xml:",chardata"`
		} `xml:",any"`
	}
	if err := xml.Unmarshal(env.Body.Response, &resp); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, f := range resp.Fields {
		out[f.XMLName.Local] = f.Value
	}
	return out, nil
}

var digestParam = regexp.MustCompile(`(\w+)="?([^",]*)"?`)

func (c *Client) digest(challenge, method, uri string) (string, error) {
	if !strings.HasPrefix(challenge, "Digest ") {
		return "", fmt.Errorf("unsupported auth challenge %q", challenge)
	}
	p := map[string]string{}
	for _, m := range digestParam.FindAllStringSubmatch(challenge[len("Digest "):], -1) {
		p[m[1]] = m[2]
	}
	cnonce := make([]byte, 8)
	if _, err := rand.Read(cnonce); err != nil {
		return "", err
	}
	cn := hex.EncodeToString(cnonce)
	const nc = "00000001"
	ha1 := md5hex(c.User + ":" + p["realm"] + ":" + c.Password)
	ha2 := md5hex(method + ":" + uri)
	response := md5hex(strings.Join([]string{ha1, p["nonce"], nc, cn, "auth", ha2}, ":"))
	return fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", algorithm=MD5, qop=auth, nc=%s, cnonce="%s", response="%s"`,
		c.User, p["realm"], p["nonce"], uri, nc, cn, response), nil
}

func md5hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

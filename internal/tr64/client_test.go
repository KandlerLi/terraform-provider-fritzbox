package tr64

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const desc = `<?xml version="1.0"?><root><device><deviceList><device><serviceList><service>
<serviceType>urn:test:service:Foo:1</serviceType><controlURL>/upnp/control/foo</controlURL>
</service></serviceList></device></deviceList></device></root>`

func newServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tr64desc.xml" {
			w.Write([]byte(desc))
			return
		}
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="HTTPS Access", nonce="abc", algorithm=MD5, qop="auth"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if !strings.Contains(auth, `username="u"`) || !strings.Contains(auth, `uri="/upnp/control/foo"`) {
			t.Errorf("unexpected Authorization header %q", auth)
		}
		if strings.Contains(r.Header.Get("SOAPACTION"), "#Missing") {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><s:Fault><detail><UPnPError><errorCode>714</errorCode><errorDescription>NoSuchEntryInArray</errorDescription></UPnPError></detail></s:Fault></s:Body></s:Envelope>`))
			return
		}
		w.Write([]byte(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:GetResponse xmlns:u="urn:test:service:Foo:1"><NewA>1</NewA><NewB>x &amp; y</NewB></u:GetResponse></s:Body></s:Envelope>`))
	}))
}

func TestCall(t *testing.T) {
	srv := newServer(t)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, User: "u", Password: "p"}

	out, err := c.Call(context.Background(), "urn:test:service:Foo:1", "Get")
	if err != nil {
		t.Fatal(err)
	}
	if out["NewA"] != "1" || out["NewB"] != "x & y" {
		t.Errorf("got %v", out)
	}

	_, err = c.Call(context.Background(), "urn:test:service:Foo:1", "Missing")
	var fault *Fault
	if !errors.As(err, &fault) || fault.Code != 714 {
		t.Errorf("want fault 714, got %v", err)
	}

	if _, err := c.Call(context.Background(), "urn:test:service:Bar:1", "Get"); err == nil {
		t.Error("want error for unknown service")
	}
}

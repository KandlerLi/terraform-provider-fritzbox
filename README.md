# terraform-provider-fritzbox

Terraform provider for AVM FRITZ!Box routers, built on the documented
[TR-064 interface](https://fritz.com/service/schnittstellen/) rather than the
web UI's internal `data.lua` endpoints, which change between FRITZ!OS releases.

Status: early. Tested against a FRITZ!Box 7530 AX (FRITZ!OS 8.25, DSL).

## Resources and data sources

- `fritzbox_port_forward`: an IPv4 port forward on the active WAN connection.
- `fritzbox_remote_access`: whether the FRITZ!Box UI is reachable from the internet.
- `fritzbox_storage`: the box's own FTP (LAN and internet) and SMB sharing.
- `data.fritzbox_port_forwards`: every port forward on the box, including
  ones Terraform does not manage (see below).

```hcl
provider "fritzbox" {}  # FRITZBOX_URL, FRITZBOX_USERNAME, FRITZBOX_PASSWORD

resource "fritzbox_port_forward" "https" {
  protocol        = "TCP"
  external_port   = 443
  internal_client = "192.168.178.100"
  internal_port   = 443
  description     = "HTTPS"
}
```

Existing forwards are imported as `<PROTOCOL>/<external_port>`:

```sh
terraform import fritzbox_port_forward.https TCP/443
```

With `import` blocks and `terraform plan -generate-config-out`, set
`provider = fritzbox` in each block; otherwise Terraform looks for
`hashicorp/fritzbox` when generating config.

Creating a forward whose protocol/port already exists on the box fails instead
of overwriting it, because TR-064's `AddPortMapping` overwrites silently.

Combine `data.fritzbox_port_forwards` with a `check` block to get a warning
on every plan when something else opened a port:

```hcl
check "no_unmanaged_port_forwards" {
  data "fritzbox_port_forwards" "all" {}

  assert {
    condition     = length(setsubtract(data.fritzbox_port_forwards.all.port_forwards[*].id, [fritzbox_port_forward.https.id])) == 0
    error_message = "Port forwards not managed by Terraform exist on the FRITZ!Box."
  }
}
```

`fritzbox_remote_access` and `fritzbox_storage` are settings that exist
exactly once per box: creating or updating them applies the setting,
destroying them only removes them from state and leaves the box unchanged.
Import them with any ID.

## FRITZ!Box setup

- *Home Network → Network → Network Settings*: enable "Allow access for applications".
- Use a dedicated FRITZ!Box user with the "FRITZ!Box Settings" permission.
  Boxes that log in with a password only still have an auto-generated user
  name (`fritzXXXX`); `http://fritz.box/login_sid.lua?version=2` lists it.

## How it talks to the box

- TR-064 is SOAP over HTTP digest auth on port 49000. The password is never
  sent in clear text, but the rest of the traffic is; keep it inside the LAN.
- DSL boxes carry port mappings on `WANPPPConnection`, cable/fiber boxes on
  `WANIPConnection`. The provider uses whichever one answers; the inactive one
  returns UPnP error 402.
- Which TR-064 services exist depends on model and firmware;
  `http://fritz.box:49000/tr64desc.xml` lists them without authentication.
- TR-064 has no IPv6 pinhole actions, so IPv6 firewall rules are out of scope.
- Whether a device may open ports itself via UPnP is a per-device setting with
  no TR-064 action; `data.fritzbox_port_forwards` detects the result instead.
  Do not turn off `X_AVM-DE_UPnP`'s status transmission: TR-064 depends on it.

## Development

```sh
go test ./...   # includes a full Terraform run against a fake FRITZ!Box
go install .
```

Use the local build via `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "KandlerLi/fritzbox" = "/home/<you>/go/bin"
  }
  direct {}
}
```

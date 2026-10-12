package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// --- data.fritzbox_device_info

type deviceInfoDataSource struct{ fb *fritzbox }

type deviceInfoModel struct {
	ManufacturerName types.String `tfsdk:"manufacturer_name"`
	ModelName        types.String `tfsdk:"model_name"`
	SoftwareVersion  types.String `tfsdk:"software_version"`
	HardwareVersion  types.String `tfsdk:"hardware_version"`
	UptimeSeconds    types.Int64  `tfsdk:"uptime_seconds"`
	ExternalIPv4     types.String `tfsdk:"external_ipv4"`
}

func newDeviceInfoDataSource() datasource.DataSource { return &deviceInfoDataSource{} }

func (d *deviceInfoDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_device_info"
}

func (d *deviceInfoDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Model, firmware and WAN address of the box. Serial number and provisioning code are deliberately left out.",
		Attributes: map[string]schema.Attribute{
			"manufacturer_name": schema.StringAttribute{Computed: true},
			"model_name":        schema.StringAttribute{Computed: true},
			"software_version":  schema.StringAttribute{Computed: true, Description: "FRITZ!OS version, e.g. `256.08.25`."},
			"hardware_version":  schema.StringAttribute{Computed: true},
			"uptime_seconds":    schema.Int64Attribute{Computed: true},
			"external_ipv4":     schema.StringAttribute{Computed: true, Description: "Public IPv4 address of the active WAN connection."},
		},
	}
}

func (d *deviceInfoDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.fb = req.ProviderData.(*fritzbox)
	}
}

func (d *deviceInfoDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	info, err := d.fb.client.GetDeviceInfo(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading device info failed", err.Error())
		return
	}
	ip, err := d.fb.client.ExternalIPv4(ctx, d.fb.wan)
	if err != nil {
		resp.Diagnostics.AddError("Reading external IP failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, deviceInfoModel{
		ManufacturerName: types.StringValue(info.ManufacturerName),
		ModelName:        types.StringValue(info.ModelName),
		SoftwareVersion:  types.StringValue(info.SoftwareVersion),
		HardwareVersion:  types.StringValue(info.HardwareVersion),
		UptimeSeconds:    types.Int64Value(int64(info.UptimeSeconds)),
		ExternalIPv4:     types.StringValue(ip),
	})...)
}

// --- data.fritzbox_hosts

type hostsDataSource struct{ fb *fritzbox }

type hostsModel struct {
	Hosts []hostEntryModel `tfsdk:"hosts"`
}

type hostEntryModel struct {
	MACAddress    types.String `tfsdk:"mac_address"`
	IPAddress     types.String `tfsdk:"ip_address"`
	HostName      types.String `tfsdk:"host_name"`
	Active        types.Bool   `tfsdk:"active"`
	InterfaceType types.String `tfsdk:"interface_type"`
	AddressSource types.String `tfsdk:"address_source"`
}

func newHostsDataSource() datasource.DataSource { return &hostsDataSource{} }

func (d *hostsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_hosts"
}

func (d *hostsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Every device the box knows, active or not. Use it to look up MAC addresses for `fritzbox_host_name`.",
		Attributes: map[string]schema.Attribute{
			"hosts": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"mac_address":    schema.StringAttribute{Computed: true},
						"ip_address":     schema.StringAttribute{Computed: true},
						"host_name":      schema.StringAttribute{Computed: true},
						"active":         schema.BoolAttribute{Computed: true},
						"interface_type": schema.StringAttribute{Computed: true, Description: "`Ethernet`, `802.11`, ..."},
						"address_source": schema.StringAttribute{Computed: true, Description: "`DHCP` or `Static`."},
					},
				},
			},
		},
	}
}

func (d *hostsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.fb = req.ProviderData.(*fritzbox)
	}
}

func (d *hostsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	hosts, err := d.fb.client.ListHosts(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Listing hosts failed", err.Error())
		return
	}
	state := hostsModel{Hosts: []hostEntryModel{}}
	for _, h := range hosts {
		state.Hosts = append(state.Hosts, hostEntryModel{
			MACAddress:    types.StringValue(h.MACAddress),
			IPAddress:     types.StringValue(h.IPAddress),
			HostName:      types.StringValue(h.HostName),
			Active:        types.BoolValue(h.Active),
			InterfaceType: types.StringValue(h.InterfaceType),
			AddressSource: types.StringValue(h.AddressSource),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

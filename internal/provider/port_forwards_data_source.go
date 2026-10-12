package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type portForwardsDataSource struct{ fb *fritzbox }

type portForwardsModel struct {
	PortForwards []portForwardEntryModel `tfsdk:"port_forwards"`
}

type portForwardEntryModel struct {
	ID             types.String `tfsdk:"id"`
	Protocol       types.String `tfsdk:"protocol"`
	ExternalPort   types.Int64  `tfsdk:"external_port"`
	InternalClient types.String `tfsdk:"internal_client"`
	InternalPort   types.Int64  `tfsdk:"internal_port"`
	Enabled        types.Bool   `tfsdk:"enabled"`
	Description    types.String `tfsdk:"description"`
	LeaseDuration  types.Int64  `tfsdk:"lease_duration"`
}

func newPortForwardsDataSource() datasource.DataSource { return &portForwardsDataSource{} }

func (d *portForwardsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_port_forwards"
}

func (d *portForwardsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Every IPv4 port forward on the active WAN connection, including ones not managed by Terraform.",
		Attributes: map[string]schema.Attribute{
			"port_forwards": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":              schema.StringAttribute{Computed: true, Description: "`<PROTOCOL>/<external_port>`, the same as `fritzbox_port_forward.id`."},
						"protocol":        schema.StringAttribute{Computed: true},
						"external_port":   schema.Int64Attribute{Computed: true},
						"internal_client": schema.StringAttribute{Computed: true},
						"internal_port":   schema.Int64Attribute{Computed: true},
						"enabled":         schema.BoolAttribute{Computed: true},
						"description":     schema.StringAttribute{Computed: true},
						"lease_duration":  schema.Int64Attribute{Computed: true, Description: "Seconds; 0 is permanent. Non-zero usually means a device opened it via UPnP."},
					},
				},
			},
		},
	}
}

func (d *portForwardsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.fb = req.ProviderData.(*fritzbox)
	}
}

func (d *portForwardsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	mappings, err := d.fb.client.ListPortMappings(ctx, d.fb.wan)
	if err != nil {
		resp.Diagnostics.AddError("Listing port forwards failed", err.Error())
		return
	}
	state := portForwardsModel{PortForwards: []portForwardEntryModel{}}
	for _, m := range mappings {
		state.PortForwards = append(state.PortForwards, portForwardEntryModel{
			ID:             types.StringValue(mappingID(m.Protocol, m.ExternalPort)),
			Protocol:       types.StringValue(m.Protocol),
			ExternalPort:   types.Int64Value(int64(m.ExternalPort)),
			InternalClient: types.StringValue(m.InternalClient),
			InternalPort:   types.Int64Value(int64(m.InternalPort)),
			Enabled:        types.BoolValue(m.Enabled),
			Description:    types.StringValue(m.Description),
			LeaseDuration:  types.Int64Value(int64(m.LeaseDuration)),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

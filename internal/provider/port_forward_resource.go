package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/KandlerLi/terraform-provider-fritzbox/internal/tr64"
)

type portForwardResource struct{ fb *fritzbox }

type portForwardModel struct {
	ID             types.String `tfsdk:"id"`
	Protocol       types.String `tfsdk:"protocol"`
	ExternalPort   types.Int64  `tfsdk:"external_port"`
	InternalClient types.String `tfsdk:"internal_client"`
	InternalPort   types.Int64  `tfsdk:"internal_port"`
	Enabled        types.Bool   `tfsdk:"enabled"`
	Description    types.String `tfsdk:"description"`
}

func newPortForwardResource() resource.Resource { return &portForwardResource{} }

func (r *portForwardResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_port_forward"
}

func (r *portForwardResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	port := []validator.Int64{int64validator.Between(1, 65535)}
	resp.Schema = schema.Schema{
		Description: "An IPv4 port forward on the FRITZ!Box's active WAN connection. Import with `<PROTOCOL>/<external_port>`, e.g. `TCP/443`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"protocol": schema.StringAttribute{
				Required:      true,
				Description:   "TCP or UDP.",
				Validators:    []validator.String{stringvalidator.OneOf("TCP", "UDP")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"external_port": schema.Int64Attribute{
				Required:      true,
				Validators:    port,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"internal_client": schema.StringAttribute{
				Required:    true,
				Description: "LAN IPv4 address the traffic is forwarded to.",
			},
			"internal_port": schema.Int64Attribute{
				Required:   true,
				Validators: port,
			},
			"enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"description": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString(""),
			},
		},
	}
}

func (r *portForwardResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.fb = req.ProviderData.(*fritzbox)
}

func (r *portForwardResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan portForwardModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// AddPortMapping silently overwrites, so refuse to take over a mapping Terraform doesn't know about.
	existing, err := r.fb.client.GetPortMapping(ctx, r.fb.wan, plan.Protocol.ValueString(), int(plan.ExternalPort.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Reading port forward failed", err.Error())
		return
	}
	if existing != nil {
		id := mappingID(existing.Protocol, existing.ExternalPort)
		resp.Diagnostics.AddError("Port forward already exists",
			fmt.Sprintf("%s already forwards to %s:%d. Import it with: terraform import <address> %s", id, existing.InternalClient, existing.InternalPort, id))
		return
	}

	if err := r.fb.client.SetPortMapping(ctx, r.fb.wan, toMapping(plan)); err != nil {
		resp.Diagnostics.AddError("Creating port forward failed", err.Error())
		return
	}
	plan.ID = types.StringValue(mappingID(plan.Protocol.ValueString(), int(plan.ExternalPort.ValueInt64())))
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *portForwardResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state portForwardModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	m, err := r.fb.client.GetPortMapping(ctx, r.fb.wan, state.Protocol.ValueString(), int(state.ExternalPort.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Reading port forward failed", err.Error())
		return
	}
	if m == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.InternalClient = types.StringValue(m.InternalClient)
	state.InternalPort = types.Int64Value(int64(m.InternalPort))
	state.Enabled = types.BoolValue(m.Enabled)
	state.Description = types.StringValue(m.Description)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *portForwardResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan portForwardModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.fb.client.SetPortMapping(ctx, r.fb.wan, toMapping(plan)); err != nil {
		resp.Diagnostics.AddError("Updating port forward failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *portForwardResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state portForwardModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.fb.client.DeletePortMapping(ctx, r.fb.wan, state.Protocol.ValueString(), int(state.ExternalPort.ValueInt64())); err != nil {
		resp.Diagnostics.AddError("Deleting port forward failed", err.Error())
	}
}

func (r *portForwardResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	protocol, portStr, ok := strings.Cut(req.ID, "/")
	port, err := strconv.Atoi(portStr)
	if !ok || err != nil || (protocol != "TCP" && protocol != "UDP") {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("want <TCP|UDP>/<external_port>, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("protocol"), protocol)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("external_port"), int64(port))...)
}

func mappingID(protocol string, externalPort int) string {
	return fmt.Sprintf("%s/%d", protocol, externalPort)
}

func toMapping(m portForwardModel) tr64.PortMapping {
	return tr64.PortMapping{
		Protocol:       m.Protocol.ValueString(),
		ExternalPort:   int(m.ExternalPort.ValueInt64()),
		InternalClient: m.InternalClient.ValueString(),
		InternalPort:   int(m.InternalPort.ValueInt64()),
		Enabled:        m.Enabled.ValueBool(),
		Description:    m.Description.ValueString(),
	}
}

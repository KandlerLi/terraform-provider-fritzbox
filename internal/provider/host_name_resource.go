package provider

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/KandlerLi/terraform-provider-fritzbox/internal/tr64"
)

var macAddress = regexp.MustCompile(`^([0-9A-F]{2}:){5}[0-9A-F]{2}$`)

type hostNameResource struct{ fb *fritzbox }

type hostNameModel struct {
	ID         types.String `tfsdk:"id"`
	MACAddress types.String `tfsdk:"mac_address"`
	HostName   types.String `tfsdk:"host_name"`
	IPAddress  types.String `tfsdk:"ip_address"`
	Active     types.Bool   `tfsdk:"active"`
}

func newHostNameResource() resource.Resource { return &hostNameResource{} }

func (r *hostNameResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_host_name"
}

func (r *hostNameResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The name of a device the box knows (Home Network → Network). The device must be known to the box. " +
			"Destroying this resource leaves the name on the box unchanged. Import with the MAC address.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mac_address": schema.StringAttribute{
				Required:      true,
				Description:   "Upper case, colon separated, e.g. `AA:BB:CC:DD:EE:FF`.",
				Validators:    []validator.String{stringvalidator.RegexMatches(macAddress, "must be upper case and colon separated, e.g. AA:BB:CC:DD:EE:FF")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"host_name": schema.StringAttribute{Required: true},
			"ip_address": schema.StringAttribute{
				Computed:    true,
				Description: "Current IPv4 address, informational.",
			},
			"active": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether the device is currently connected, informational.",
			},
		},
	}
}

func (r *hostNameResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.fb = req.ProviderData.(*fritzbox)
	}
}

// set writes the name and returns the device as read back; "unknown device" is an error, not a removal.
func (r *hostNameResource) set(ctx context.Context, plan hostNameModel) (hostNameModel, error) {
	mac := plan.MACAddress.ValueString()
	if err := r.fb.client.SetHostName(ctx, mac, plan.HostName.ValueString()); err != nil {
		return plan, err
	}
	h, err := r.fb.client.GetHost(ctx, mac)
	if err != nil {
		return plan, err
	}
	if h == nil {
		return plan, fmt.Errorf("the box does not know a device with MAC address %s", mac)
	}
	return hostModel(h), nil
}

func hostModel(h *tr64.Host) hostNameModel {
	return hostNameModel{
		ID:         types.StringValue(h.MACAddress),
		MACAddress: types.StringValue(h.MACAddress),
		HostName:   types.StringValue(h.HostName),
		IPAddress:  types.StringValue(h.IPAddress),
		Active:     types.BoolValue(h.Active),
	}
}

func (r *hostNameResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan hostNameModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.set(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Setting host name failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *hostNameResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state hostNameModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	h, err := r.fb.client.GetHost(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading host failed", err.Error())
		return
	}
	if h == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, hostModel(h))...)
}

func (r *hostNameResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan hostNameModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.set(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Setting host name failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *hostNameResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {
}

func (r *hostNameResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !macAddress.MatchString(req.ID) {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("want an upper case, colon separated MAC address, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

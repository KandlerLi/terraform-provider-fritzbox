package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type wanAccessBlockResource struct{ fb *fritzbox }

type wanAccessBlockModel struct {
	ID        types.String `tfsdk:"id"`
	IPAddress types.String `tfsdk:"ip_address"`
}

func newWANAccessBlockResource() resource.Resource { return &wanAccessBlockResource{} }

func (r *wanAccessBlockResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_wan_access_block"
}

func (r *wanAccessBlockResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Blocks internet access for one LAN IPv4 address (the box's \"block internet use\" switch). " +
			"Destroying it lifts the block. The box keys this by IP, so give the device a fixed address. Import with the IP.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"ip_address": schema.StringAttribute{
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
		},
	}
}

func (r *wanAccessBlockResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.fb = req.ProviderData.(*fritzbox)
	}
}

func (r *wanAccessBlockResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan wanAccessBlockModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ip := plan.IPAddress.ValueString()
	if err := r.fb.client.SetWANAccessBlocked(ctx, ip, true); err != nil {
		resp.Diagnostics.AddError("Blocking internet access failed", err.Error())
		return
	}
	blocked, err := r.fb.client.WANAccessBlocked(ctx, ip)
	if err != nil || !blocked {
		resp.Diagnostics.AddError("Blocking internet access failed", fmt.Sprintf("the box does not report %s as blocked (error: %v)", ip, err))
		return
	}
	plan.ID = types.StringValue(ip)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *wanAccessBlockResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state wanAccessBlockModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	blocked, err := r.fb.client.WANAccessBlocked(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading internet access failed", err.Error())
		return
	}
	if !blocked {
		resp.State.RemoveResource(ctx)
		return
	}
	state.IPAddress = state.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *wanAccessBlockResource) Update(context.Context, resource.UpdateRequest, *resource.UpdateResponse) {
	// ip_address forces replacement, so there is nothing to update in place.
}

func (r *wanAccessBlockResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state wanAccessBlockModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.fb.client.SetWANAccessBlocked(ctx, state.ID.ValueString(), false); err != nil {
		resp.Diagnostics.AddError("Lifting internet block failed", err.Error())
	}
}

func (r *wanAccessBlockResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

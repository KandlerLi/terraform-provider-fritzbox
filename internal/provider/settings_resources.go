package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/KandlerLi/terraform-provider-fritzbox/internal/tr64"
)

// Settings exist exactly once per box, so create/update both just apply them and
// destroy only drops them from state: there is no "absent" setting to go back to.
const settingsDestroyNote = " Destroying this resource leaves the setting on the box unchanged."

func settingsID(id string) schema.StringAttribute {
	return schema.StringAttribute{
		Computed:      true,
		Description:   "Always `" + id + "`; any value works as import ID.",
		PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

// --- fritzbox_remote_access

type remoteAccessResource struct{ fb *fritzbox }

type remoteAccessModel struct {
	ID      types.String `tfsdk:"id"`
	Enabled types.Bool   `tfsdk:"enabled"`
	Port    types.Int64  `tfsdk:"port"`
}

func newRemoteAccessResource() resource.Resource { return &remoteAccessResource{} }

func (r *remoteAccessResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_remote_access"
}

func (r *remoteAccessResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Whether the FRITZ!Box UI is reachable from the internet (Internet → Permit Access → FRITZ!Box Services)." + settingsDestroyNote,
		Attributes: map[string]schema.Attribute{
			"id":      settingsID("remote_access"),
			"enabled": schema.BoolAttribute{Required: true},
			"port": schema.Int64Attribute{
				Computed:      true,
				Description:   "HTTPS port of the remote UI, as configured on the box.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *remoteAccessResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.fb = req.ProviderData.(*fritzbox)
	}
}

func (r *remoteAccessResource) apply(ctx context.Context, plan remoteAccessModel) (remoteAccessModel, error) {
	if err := r.fb.client.SetRemoteAccessEnabled(ctx, plan.Enabled.ValueBool()); err != nil {
		return plan, err
	}
	return r.read(ctx)
}

func (r *remoteAccessResource) read(ctx context.Context) (remoteAccessModel, error) {
	ra, err := r.fb.client.GetRemoteAccess(ctx)
	if err != nil {
		return remoteAccessModel{}, err
	}
	return remoteAccessModel{
		ID:      types.StringValue("remote_access"),
		Enabled: types.BoolValue(ra.Enabled),
		Port:    types.Int64Value(int64(ra.Port)),
	}, nil
}

func (r *remoteAccessResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan remoteAccessModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.apply(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Setting remote access failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *remoteAccessResource) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	state, err := r.read(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading remote access failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *remoteAccessResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan remoteAccessModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.apply(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Setting remote access failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *remoteAccessResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {
}

func (r *remoteAccessResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), "remote_access")...)
}

// --- fritzbox_storage

type storageResource struct{ fb *fritzbox }

type storageModel struct {
	ID            types.String `tfsdk:"id"`
	FTPEnabled    types.Bool   `tfsdk:"ftp_enabled"`
	FTPWANEnabled types.Bool   `tfsdk:"ftp_wan_enabled"`
	FTPWANSSLOnly types.Bool   `tfsdk:"ftp_wan_ssl_only"`
	SMBEnabled    types.Bool   `tfsdk:"smb_enabled"`
}

func newStorageResource() resource.Resource { return &storageResource{} }

func (r *storageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_storage"
}

func (r *storageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The FRITZ!Box's own NAS sharing (Home Network → USB / Storage)." + settingsDestroyNote,
		Attributes: map[string]schema.Attribute{
			"id":               settingsID("storage"),
			"ftp_enabled":      schema.BoolAttribute{Required: true, Description: "FTP access from the LAN."},
			"ftp_wan_enabled":  schema.BoolAttribute{Required: true, Description: "FTP access from the internet."},
			"ftp_wan_ssl_only": schema.BoolAttribute{Required: true, Description: "Require TLS for FTP from the internet."},
			"smb_enabled":      schema.BoolAttribute{Required: true, Description: "SMB (Windows file sharing) in the LAN."},
		},
	}
}

func (r *storageResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.fb = req.ProviderData.(*fritzbox)
	}
}

func (r *storageResource) apply(ctx context.Context, plan storageModel) (storageModel, error) {
	err := r.fb.client.SetStorage(ctx, tr64.Storage{
		FTPEnabled:    plan.FTPEnabled.ValueBool(),
		FTPWANEnabled: plan.FTPWANEnabled.ValueBool(),
		FTPWANSSLOnly: plan.FTPWANSSLOnly.ValueBool(),
		SMBEnabled:    plan.SMBEnabled.ValueBool(),
	})
	if err != nil {
		return plan, err
	}
	return r.read(ctx)
}

func (r *storageResource) read(ctx context.Context) (storageModel, error) {
	s, err := r.fb.client.GetStorage(ctx)
	if err != nil {
		return storageModel{}, err
	}
	return storageModel{
		ID:            types.StringValue("storage"),
		FTPEnabled:    types.BoolValue(s.FTPEnabled),
		FTPWANEnabled: types.BoolValue(s.FTPWANEnabled),
		FTPWANSSLOnly: types.BoolValue(s.FTPWANSSLOnly),
		SMBEnabled:    types.BoolValue(s.SMBEnabled),
	}, nil
}

func (r *storageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan storageModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.apply(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Setting storage sharing failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *storageResource) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	state, err := r.read(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading storage sharing failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *storageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan storageModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.apply(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Setting storage sharing failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *storageResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {}

func (r *storageResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), "storage")...)
}

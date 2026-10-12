package provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/KandlerLi/terraform-provider-fritzbox/internal/tr64"
)

type dynamicDNSResource struct{ fb *fritzbox }

type dynamicDNSModel struct {
	ID                   types.String `tfsdk:"id"`
	Enabled              types.Bool   `tfsdk:"enabled"`
	ProviderName         types.String `tfsdk:"provider_name"`
	UpdateURL            types.String `tfsdk:"update_url"`
	Domain               types.String `tfsdk:"domain"`
	Mode                 types.String `tfsdk:"mode"`
	ServerIPv4           types.String `tfsdk:"server_ipv4"`
	ServerIPv6           types.String `tfsdk:"server_ipv6"`
	UsernameWO           types.String `tfsdk:"username_wo"`
	PasswordWO           types.String `tfsdk:"password_wo"`
	CredentialsWOVersion types.Int64  `tfsdk:"credentials_wo_version"`
	StatusIPv4           types.String `tfsdk:"status_ipv4"`
	StatusIPv6           types.String `tfsdk:"status_ipv6"`
}

func newDynamicDNSResource() resource.Resource { return &dynamicDNSResource{} }

func (r *dynamicDNSResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dynamic_dns"
}

func (r *dynamicDNSResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	writeOnly := func(description string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Sensitive: true, WriteOnly: true, Description: description}
	}
	resp.Schema = schema.Schema{
		Description: "The FRITZ!Box's DynDNS client (Internet → Permit Access → DynDNS). " +
			"Credentials are write-only and never stored in state; every change needs them, because the box " +
			"cannot update the config without overwriting the stored password. Requires Terraform 1.11 or newer." + settingsDestroyNote,
		Attributes: map[string]schema.Attribute{
			"id":            settingsID("dynamic_dns"),
			"enabled":       schema.BoolAttribute{Required: true},
			"provider_name": schema.StringAttribute{Required: true, Description: "As listed by the box's DynDNS provider dropdown, or `userdefined`."},
			"update_url":    schema.StringAttribute{Required: true, Description: "Update URL with the box's `<username>`, `<pass>`, `<domain>`, `<ipaddr>` placeholders."},
			"domain":        schema.StringAttribute{Required: true},
			"mode": schema.StringAttribute{
				Required:   true,
				Validators: []validator.String{stringvalidator.OneOf("ddns_v4", "ddns_v6", "ddns_both", "ddns_together")},
			},
			"server_ipv4":            schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("")},
			"server_ipv6":            schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("")},
			"username_wo":            writeOnly("DynDNS username. Required whenever this resource is created or changed."),
			"password_wo":            writeOnly("DynDNS password. Required whenever this resource is created or changed."),
			"credentials_wo_version": schema.Int64Attribute{Optional: true, Description: "Change this to push new credentials when nothing else changed."},
			"status_ipv4":            schema.StringAttribute{Computed: true, Description: "Last update result as reported by the box."},
			"status_ipv6":            schema.StringAttribute{Computed: true},
		},
	}
}

func (r *dynamicDNSResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.fb = req.ProviderData.(*fritzbox)
	}
}

func (r *dynamicDNSResource) read(ctx context.Context, keep dynamicDNSModel) (dynamicDNSModel, error) {
	d, err := r.fb.client.GetDynamicDNS(ctx)
	if err != nil {
		return dynamicDNSModel{}, err
	}
	return dynamicDNSModel{
		ID:                   types.StringValue("dynamic_dns"),
		Enabled:              types.BoolValue(d.Enabled),
		ProviderName:         types.StringValue(d.ProviderName),
		UpdateURL:            types.StringValue(d.UpdateURL),
		Domain:               types.StringValue(d.Domain),
		Mode:                 types.StringValue(d.Mode),
		ServerIPv4:           types.StringValue(d.ServerIPv4),
		ServerIPv6:           types.StringValue(d.ServerIPv6),
		UsernameWO:           types.StringNull(),
		PasswordWO:           types.StringNull(),
		CredentialsWOVersion: keep.CredentialsWOVersion,
		StatusIPv4:           types.StringValue(d.StatusIPv4),
		StatusIPv6:           types.StringValue(d.StatusIPv6),
	}, nil
}

// apply reads the write-only credentials from config, since plan and state never hold them.
func (r *dynamicDNSResource) apply(ctx context.Context, plan, config dynamicDNSModel) (dynamicDNSModel, error) {
	if config.UsernameWO.IsNull() || config.PasswordWO.IsNull() {
		return plan, errors.New("username_wo and password_wo are required to change the DynDNS config: " +
			"the box's SetDDNSConfig always overwrites the stored credentials")
	}
	err := r.fb.client.SetDynamicDNS(ctx, tr64.DynamicDNS{
		Enabled:      plan.Enabled.ValueBool(),
		ProviderName: plan.ProviderName.ValueString(),
		UpdateURL:    plan.UpdateURL.ValueString(),
		Domain:       plan.Domain.ValueString(),
		Mode:         plan.Mode.ValueString(),
		ServerIPv4:   plan.ServerIPv4.ValueString(),
		ServerIPv6:   plan.ServerIPv6.ValueString(),
	}, config.UsernameWO.ValueString(), config.PasswordWO.ValueString())
	if err != nil {
		return plan, err
	}
	return r.read(ctx, plan)
}

func (r *dynamicDNSResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config dynamicDNSModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.apply(ctx, plan, config)
	if err != nil {
		resp.Diagnostics.AddError("Setting DynDNS failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *dynamicDNSResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var prior dynamicDNSModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.read(ctx, prior)
	if err != nil {
		resp.Diagnostics.AddError("Reading DynDNS failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *dynamicDNSResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, config dynamicDNSModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.apply(ctx, plan, config)
	if err != nil {
		resp.Diagnostics.AddError("Setting DynDNS failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *dynamicDNSResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {
}

func (r *dynamicDNSResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), "dynamic_dns")...)
}

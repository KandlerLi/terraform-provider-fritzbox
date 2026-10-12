package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/KandlerLi/terraform-provider-fritzbox/internal/tr64"
)

type wlanResource struct{ fb *fritzbox }

type wlanModel struct {
	ID                  types.String `tfsdk:"id"`
	Index               types.Int64  `tfsdk:"index"`
	Enabled             types.Bool   `tfsdk:"enabled"`
	SSID                types.String `tfsdk:"ssid"`
	SSIDBroadcast       types.Bool   `tfsdk:"ssid_broadcast"`
	Security            types.String `tfsdk:"security"`
	PassphraseWO        types.String `tfsdk:"passphrase_wo"`
	PassphraseWOVersion types.Int64  `tfsdk:"passphrase_wo_version"`
	FrequencyBand       types.String `tfsdk:"frequency_band"`
	APType              types.String `tfsdk:"ap_type"`
}

func newWLANResource() resource.Resource { return &wlanResource{} }

func (r *wlanResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_wlan"
}

func (r *wlanResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	computed := func(description string) schema.StringAttribute {
		return schema.StringAttribute{
			Computed:      true,
			Description:   description,
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		}
	}
	resp.Schema = schema.Schema{
		Description: "One WLAN (radio/SSID) of the box. Changing it can drop the connection of clients using it, " +
			"including the machine running Terraform." + settingsDestroyNote + " Import with the index.",
		Attributes: map[string]schema.Attribute{
			"id": computed(""),
			"index": schema.Int64Attribute{
				Required:      true,
				Description:   "TR-064 WLANConfiguration index: typically 1 = 2.4 GHz, 2 = 5 GHz, 3 = guest. Check `frequency_band` and `ap_type`.",
				Validators:    []validator.Int64{int64validator.Between(1, 4)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"enabled": schema.BoolAttribute{Required: true},
			"ssid":    schema.StringAttribute{Required: true},
			"ssid_broadcast": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Whether the SSID is visible. Defaults to `true`, the FRITZ!OS factory setting.",
			},
			"security": schema.StringAttribute{
				Required:    true,
				Description: "TR-064 beacon type: `11i` is WPA2, `11iandWPA3` WPA2+WPA3, `WPA3`, `None` (open), ...",
				Validators: []validator.String{stringvalidator.OneOf(
					"None", "Basic", "WPA", "11i", "WPAand11i", "WPA3", "11iandWPA3", "OWE", "OWETrans")},
			},
			"passphrase_wo": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				WriteOnly:   true,
				Description: "WPA passphrase. Sent on create and whenever `passphrase_wo_version` changes; never read back.",
			},
			"passphrase_wo_version": schema.Int64Attribute{Optional: true, Description: "Change this to push a new passphrase."},
			"frequency_band":        computed("2400, 5000 or 6000."),
			"ap_type":               computed("`normal` or `guest`."),
		},
	}
}

func (r *wlanResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.fb = req.ProviderData.(*fritzbox)
	}
}

func (r *wlanResource) read(ctx context.Context, index int64, version types.Int64) (wlanModel, error) {
	w, err := r.fb.client.GetWLAN(ctx, int(index))
	if err != nil {
		return wlanModel{}, err
	}
	return wlanModel{
		ID:                  types.StringValue(strconv.FormatInt(index, 10)),
		Index:               types.Int64Value(index),
		Enabled:             types.BoolValue(w.Enabled),
		SSID:                types.StringValue(w.SSID),
		SSIDBroadcast:       types.BoolValue(w.SSIDBroadcast),
		Security:            types.StringValue(w.Security),
		PassphraseWO:        types.StringNull(),
		PassphraseWOVersion: version,
		FrequencyBand:       types.StringValue(w.FrequencyBand),
		APType:              types.StringValue(w.APType),
	}, nil
}

func (r *wlanResource) apply(ctx context.Context, plan, config wlanModel, sendPassphrase bool) (wlanModel, error) {
	index := int(plan.Index.ValueInt64())
	if sendPassphrase {
		if config.PassphraseWO.IsNull() {
			return plan, errors.New("passphrase_wo_version changed but passphrase_wo is not set")
		}
		if err := r.fb.client.SetWLANPassphrase(ctx, index, config.PassphraseWO.ValueString()); err != nil {
			return plan, err
		}
	}
	current, err := r.fb.client.GetWLAN(ctx, index)
	if err != nil {
		return plan, err
	}
	err = r.fb.client.SetWLAN(ctx, index, current, tr64.WLAN{
		Enabled:       plan.Enabled.ValueBool(),
		SSID:          plan.SSID.ValueString(),
		SSIDBroadcast: plan.SSIDBroadcast.ValueBool(),
		Security:      plan.Security.ValueString(),
	})
	if err != nil {
		return plan, err
	}
	return r.read(ctx, plan.Index.ValueInt64(), plan.PassphraseWOVersion)
}

func (r *wlanResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config wlanModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.apply(ctx, plan, config, !config.PassphraseWO.IsNull())
	if err != nil {
		resp.Diagnostics.AddError("Setting WLAN failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *wlanResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var prior wlanModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	index := prior.Index.ValueInt64()
	if prior.Index.IsNull() { // right after import, only id is set
		var err error
		if index, err = strconv.ParseInt(prior.ID.ValueString(), 10, 64); err != nil {
			resp.Diagnostics.AddError("Invalid WLAN id", err.Error())
			return
		}
	}
	state, err := r.read(ctx, index, prior.PassphraseWOVersion)
	if err != nil {
		resp.Diagnostics.AddError("Reading WLAN failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *wlanResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, config, prior wlanModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.apply(ctx, plan, config, !plan.PassphraseWOVersion.Equal(prior.PassphraseWOVersion))
	if err != nil {
		resp.Diagnostics.AddError("Setting WLAN failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *wlanResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {}

func (r *wlanResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if i, err := strconv.Atoi(req.ID); err != nil || i < 1 || i > 4 {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("want a WLAN index 1-4, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

package provider

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/KandlerLi/terraform-provider-fritzbox/internal/tr64"
)

// fritzbox is what resources receive from Configure.
type fritzbox struct {
	client *tr64.Client
	wan    string
}

type fritzboxProvider struct{ version string }

type providerModel struct {
	URL      types.String `tfsdk:"url"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &fritzboxProvider{version: version} }
}

func (p *fritzboxProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "fritzbox"
	resp.Version = p.version
}

func (p *fritzboxProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a FRITZ!Box router through its TR-064 interface.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Optional:    true,
				Description: "TR-064 base URL. Defaults to FRITZBOX_URL, then http://fritz.box:49000.",
			},
			"username": schema.StringAttribute{
				Optional:    true,
				Description: "FRITZ!Box user. Defaults to FRITZBOX_USERNAME.",
			},
			"password": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "FRITZ!Box password. Defaults to FRITZBOX_PASSWORD.",
			},
		},
	}
}

func (p *fritzboxProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	url := valueOrEnv(cfg.URL, "FRITZBOX_URL", "http://fritz.box:49000")
	username := valueOrEnv(cfg.Username, "FRITZBOX_USERNAME", "")
	password := valueOrEnv(cfg.Password, "FRITZBOX_PASSWORD", "")
	if username == "" || password == "" {
		resp.Diagnostics.AddError("Missing credentials", "Set username and password, or FRITZBOX_USERNAME and FRITZBOX_PASSWORD.")
		return
	}

	client := &tr64.Client{
		BaseURL:  url,
		User:     username,
		Password: password,
		HTTP:     &http.Client{Timeout: 30 * time.Second},
	}
	wan, err := client.ActiveWANService(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Cannot reach FRITZ!Box", err.Error())
		return
	}

	fb := &fritzbox{client: client, wan: wan}
	resp.ResourceData = fb
	resp.DataSourceData = fb
}

func (p *fritzboxProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{newPortForwardResource, newRemoteAccessResource, newStorageResource, newDynamicDNSResource, newHostNameResource, newWLANResource, newWANAccessBlockResource}
}

func (p *fritzboxProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{newPortForwardsDataSource, newDeviceInfoDataSource, newHostsDataSource}
}

func valueOrEnv(v types.String, env, fallback string) string {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueString()
	}
	if e := os.Getenv(env); e != "" {
		return e
	}
	return fallback
}

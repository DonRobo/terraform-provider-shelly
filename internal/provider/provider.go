package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = &ShellyProvider{}

// ShellyProvider defines the provider implementation.
type ShellyProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// ShellyProviderModel describes the provider data model.
type ShellyProviderModel struct {
	Password types.String `tfsdk:"password"`
}

func (p *ShellyProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "shelly"
	resp.Version = p.version
}

func (p *ShellyProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The Shelly provider allows management and configuration of Shelly Gen2 devices via their local API.",
		Attributes: map[string]schema.Attribute{
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "The password of the Shelly devices (HTTP digest authentication, user `admin`). The provider uses it for each device that has authentication on. You can also set it with the `SHELLY_PASSWORD` environment variable. A resource `device_password` overrides this value.",
			},
		},
	}
}

func (p *ShellyProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data ShellyProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...) // get config

	if resp.Diagnostics.HasError() {
		return
	}

	pd := &providerData{password: resolvePassword(data.Password)}
	resp.ResourceData = pd
	resp.DataSourceData = pd
}

func (p *ShellyProvider) Resources(ctx context.Context) []func() resource.Resource {
	return append(generatedConfigResources(), NewAuthResource)
}

func (p *ShellyProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewShellyDeviceDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &ShellyProvider{
			version: version,
		}
	}
}

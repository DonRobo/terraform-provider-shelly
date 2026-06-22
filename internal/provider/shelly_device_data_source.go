package provider

import (
	"context"

	shelly "github.com/DonRobo/shelly-go"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"resty.dev/v3"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &ShellyDeviceDataSource{}

type ShellyDeviceDataSource struct {
}

type ShellyDeviceModel struct {
	IP      types.String `tfsdk:"ip"`
	MAC     types.String `tfsdk:"mac"`
	Version types.String `tfsdk:"version"`
}

func NewShellyDeviceDataSource() datasource.DataSource {
	return &ShellyDeviceDataSource{}
}

func (d *ShellyDeviceDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "shelly_device"
}

func (d *ShellyDeviceDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The shelly_device data source allows you to query basic information (firmware version, MAC address) from a Shelly device on your network.",
		Attributes: map[string]schema.Attribute{
			"ip": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The IP address of the device.",
			},
			"version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The firmware version of the device.",
			},
			"mac": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The MAC address of the device.",
			},
		},
	}
}

func (d *ShellyDeviceDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
}

func (d *ShellyDeviceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	data := &ShellyDeviceModel{}

	diags := req.Config.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := resty.New()
	defer client.Close()
	client.SetBaseURL("http://" + data.IP.ValueString())

	info, _, err := (&shelly.ShellyGetDeviceInfoRequest{}).Do(client)
	if err != nil {
		resp.Diagnostics.AddError("Failed to query device info", err.Error())
		return
	}

	if info.Ver != nil && *info.Ver != "" {
		data.Version = types.StringValue(*info.Ver)
	} else {
		resp.Diagnostics.AddError("Version not found", "Could not find valid firmware version in response.")
	}

	if info.MAC != nil && *info.MAC != "" {
		data.MAC = types.StringValue(*info.MAC)
	} else {
		resp.Diagnostics.AddError("MAC address not found", "Could not find valid MAC address in response.")
	}

	// Write to state
	diags = resp.State.Set(ctx, data)
	resp.Diagnostics.Append(diags...)
}

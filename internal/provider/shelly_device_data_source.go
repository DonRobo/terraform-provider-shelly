package provider

import (
	"context"
	"fmt"

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

	ip := data.IP.ValueString()

	// Detect device version first
	version := DetectDeviceVersion(ip)
	if version.Generation == 1 {
		settings, err := gen1GetSettings(ip)
		if err != nil {
			resp.Diagnostics.AddError("Failed to query Gen1 device info", err.Error())
			return
		}
		if settings.Device.MAC != "" {
			data.MAC = types.StringValue(settings.Device.MAC)
		} else if version.MAC != "" {
			data.MAC = types.StringValue(version.MAC)
		} else {
			data.MAC = types.StringNull()
		}
		if settings.Fw != "" {
			data.Version = types.StringValue(settings.Fw)
		} else if version.Firmware != "" {
			data.Version = types.StringValue(version.Firmware)
		} else {
			data.Version = types.StringNull()
		}

		diags = resp.State.Set(ctx, data)
		resp.Diagnostics.Append(diags...)
		return
	}

	if version.Generation != 2 {
		msg := BuildDeviceCompatibilityError(ip, version)
		resp.Diagnostics.AddError("Unsupported Device", msg)
		return
	}

	client := resty.New()
	defer client.Close()
	client.SetBaseURL("http://" + ip)

	info, _, err := (&shelly.ShellyGetDeviceInfoRequest{}).Do(client)
	if err != nil {
		resp.Diagnostics.AddError("Failed to query device info",
			fmt.Sprintf("Device at %s may not be a Gen2 device. Error: %v", ip, err))
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

package provider

import (
	"context"

	shelly "github.com/DonRobo/shelly-go"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"resty.dev/v3"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &sysConfigResource{}
	_ resource.ResourceWithImportState = &sysConfigResource{}
)

func NewSysConfigResource() resource.Resource {
	return &sysConfigResource{}
}

type sysConfigResourceModel struct {
	IP      types.String `tfsdk:"ip"`
	Name    types.String `tfsdk:"name"`
	EcoMode types.Bool   `tfsdk:"eco_mode"`
}

type sysConfigResource struct {
}

func (c *sysConfigResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sys_config"
}

func (c *sysConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"ip": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The IP address of the Shelly device.",
			},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The name of the Shelly device.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"eco_mode": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Eco mode (experimental) decreases power consumption when enabled, at the cost of reduced execution speed and increased network latency.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (c *sysConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sysConfigResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	statusReq := &shelly.SysGetConfigRequest{}

	client := resty.New()
	defer client.Close()
	client.SetBaseURL("http://" + state.IP.ValueString())

	statusResp, _, err := statusReq.Do(client)
	if err != nil {
		resp.Diagnostics.AddError("Failed to query device status", err.Error())
		return
	}

	if statusResp.Device.Name == nil {
		state.Name = types.StringNull()
	} else {
		state.Name = types.StringValue(*statusResp.Device.Name)
	}

	state.EcoMode = types.BoolValue(statusResp.Device.EcoMode)

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func setSysConfig(plan sysConfigResourceModel, diags *diag.Diagnostics) error {
	var sysConfig shelly.SysDeviceConfig
	if !plan.Name.IsNull() && !plan.Name.IsUnknown() {
		nameStr := plan.Name.ValueString()
		sysConfig.Name = &nameStr
	}
	sysConfig.EcoMode = plan.EcoMode.ValueBool()

	statusReq := &shelly.SysSetConfigRequest{
		Config: shelly.SysConfig{
			Device: &sysConfig,
		},
	}

	client := resty.New()
	defer client.Close()
	client.SetBaseURL("http://" + plan.IP.ValueString())

	_, _, err := statusReq.Do(client)
	if err != nil {
		diags.AddError("Failed to set device configuration", err.Error())
		return err
	}

	return nil
}

func (c *sysConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan sysConfigResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := setSysConfig(plan, &resp.Diagnostics); err != nil {
		return
	}
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (c *sysConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan sysConfigResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := setSysConfig(plan, &resp.Diagnostics); err != nil {
		return
	}
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (c *sysConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("ip"), req, resp)
}

func (c *sysConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.State.RemoveResource(ctx)
}

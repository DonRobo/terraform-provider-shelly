package provider

import (
	"context"
	"net/url"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &coiotConfigResource{}
	_ resource.ResourceWithImportState = &coiotConfigResource{}
)

func NewCoIoTConfigResource() resource.Resource { return &coiotConfigResource{} }

type coiotConfigResource struct{}

type coiotConfigResourceModel struct {
	IP           types.String  `tfsdk:"ip"`
	Enable       types.Bool    `tfsdk:"enable"`
	Peer         types.String  `tfsdk:"peer"`
	UpdatePeriod types.Float64 `tfsdk:"update_period"`
}

func (r *coiotConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_coiot_config"
}

func (r *coiotConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"ip": schema.StringAttribute{Required: true, MarkdownDescription: "The IP address of the Shelly device."},
			"enable": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "True if CoIoT is enabled, false otherwise",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"peer": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "CoIoT peer endpoint in host:port format",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"update_period": schema.Float64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "CoIoT update period in seconds",
				PlanModifiers:       []planmodifier.Float64{float64planmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *coiotConfigResource) get(_ context.Context, m *coiotConfigResourceModel, diags *diag.Diagnostics) {
	version := DetectDeviceVersion(m.IP.ValueString())
	if version.Generation == 2 {
		diags.AddError("Unsupported Device", "`shelly_coiot_config` currently supports Gen1 devices only.")
		return
	}

	got, err := gen1GetSettings(m.IP.ValueString())
	if err != nil {
		diags.AddError("Failed to read Gen1 CoIoT config", err.Error())
		return
	}

	if got.Coiot.Enabled != nil {
		m.Enable = types.BoolValue(*got.Coiot.Enabled)
	} else {
		m.Enable = types.BoolNull()
	}
	if got.Coiot.Peer != "" {
		m.Peer = types.StringValue(got.Coiot.Peer)
	} else {
		m.Peer = types.StringNull()
	}
	if got.Coiot.UpdatePeriod != nil {
		m.UpdatePeriod = types.Float64Value(float64(*got.Coiot.UpdatePeriod))
	} else {
		m.UpdatePeriod = types.Float64Null()
	}
}

func (r *coiotConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state coiotConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.get(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *coiotConfigResource) apply(_ context.Context, plan coiotConfigResourceModel, diags *diag.Diagnostics) {
	version := DetectDeviceVersion(plan.IP.ValueString())
	if version.Generation == 2 {
		diags.AddError("Unsupported Device", "`shelly_coiot_config` currently supports Gen1 devices only.")
		return
	}

	q := url.Values{}
	if !plan.Enable.IsNull() && !plan.Enable.IsUnknown() {
		q.Set("coiot_enable", strconv.FormatBool(plan.Enable.ValueBool()))
	}
	if !plan.Peer.IsNull() && !plan.Peer.IsUnknown() {
		q.Set("coiot_peer", plan.Peer.ValueString())
	}
	if !plan.UpdatePeriod.IsNull() && !plan.UpdatePeriod.IsUnknown() {
		q.Set("coiot_update_period", strconv.FormatInt(int64(plan.UpdatePeriod.ValueFloat64()), 10))
	}
	if err := gen1SetSettings(plan.IP.ValueString(), q); err != nil {
		diags.AddError("Failed to set Gen1 CoIoT config", err.Error())
	}
}

func (r *coiotConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan coiotConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	r.get(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *coiotConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan coiotConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	r.get(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *coiotConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.State.RemoveResource(ctx)
}

func (r *coiotConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("ip"), req, resp)
}

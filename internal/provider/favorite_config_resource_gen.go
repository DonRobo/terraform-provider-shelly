package provider

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &favoriteConfigResource{}
	_ resource.ResourceWithImportState = &favoriteConfigResource{}
)

func NewFavoriteConfigResource() resource.Resource { return &favoriteConfigResource{} }

type favoriteConfigResource struct{}

type favoriteConfigResourceModel struct {
	IP   types.String `tfsdk:"ip"`
	ID   types.Int64  `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Pos  types.Int64  `tfsdk:"pos"`
}

func (r *favoriteConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_favorite_config"
}

func (r *favoriteConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"ip": schema.StringAttribute{Required: true, MarkdownDescription: "The IP address of the Shelly device."},
			"id": schema.Int64Attribute{Required: true, MarkdownDescription: "Favorite index (0-based)."},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Favorite label",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"pos": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Favorite roller position",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *favoriteConfigResource) get(_ context.Context, m *favoriteConfigResourceModel, diags *diag.Diagnostics) {
	version := DetectDeviceVersion(m.IP.ValueString())
	if version.Generation == 2 {
		diags.AddError("Unsupported Device", "`shelly_favorite_config` currently supports Gen1 devices only.")
		return
	}

	got, err := gen1GetFavoriteSettings(m.IP.ValueString(), int(m.ID.ValueInt64()))
	if err != nil {
		diags.AddError("Failed to read Gen1 favorite config", err.Error())
		return
	}

	m.Name = types.StringValue(got.Name)
	if got.Pos != nil {
		m.Pos = types.Int64Value(*got.Pos)
	} else {
		m.Pos = types.Int64Null()
	}
}

func (r *favoriteConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state favoriteConfigResourceModel
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

func (r *favoriteConfigResource) apply(_ context.Context, plan favoriteConfigResourceModel, diags *diag.Diagnostics) {
	version := DetectDeviceVersion(plan.IP.ValueString())
	if version.Generation == 2 {
		diags.AddError("Unsupported Device", "`shelly_favorite_config` currently supports Gen1 devices only.")
		return
	}

	q := url.Values{}
	if !plan.Name.IsNull() && !plan.Name.IsUnknown() {
		q.Set("name", plan.Name.ValueString())
	}
	if !plan.Pos.IsNull() && !plan.Pos.IsUnknown() {
		q.Set("pos", strconv.FormatInt(plan.Pos.ValueInt64(), 10))
	}
	if err := gen1SetFavoriteSettings(plan.IP.ValueString(), int(plan.ID.ValueInt64()), q); err != nil {
		diags.AddError("Failed to set Gen1 favorite config", err.Error())
	}
}

func (r *favoriteConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan favoriteConfigResourceModel
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

func (r *favoriteConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan favoriteConfigResourceModel
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

func (r *favoriteConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.State.RemoveResource(ctx)
}

func (r *favoriteConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected format ip:id")
		return
	}
	id, err := strconv.Atoi(parts[1])
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("id must be an integer: %v", err))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ip"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

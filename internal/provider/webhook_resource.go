// Hand-written: Webhook is not a GetConfig/SetConfig component, so
// internal/provider/gen doesn't touch it. See gen/main.go's comment on
// why a hand-written NewXConfigResource would collide - this one is named
// NewWebhookResource specifically to avoid that collision, and is
// registered directly in provider.go rather than resources_gen.go.
//
// Unlike the generated *_config resources (which adopt whatever's on the
// device and whose Delete is a no-op RemoveResource - see any
// *_config_resource_gen.go), a webhook is a real object the device
// allocates an id for on Create and forgets on Delete. This resource's
// Delete calls Webhook.Delete for real.
package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/DonRobo/shelly-go/rpc"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"resty.dev/v3"
)

var (
	_ resource.Resource                = &webhookResource{}
	_ resource.ResourceWithImportState = &webhookResource{}
)

func NewWebhookResource() resource.Resource { return &webhookResource{} }

type webhookResource struct{}

type webhookResourceModel struct {
	IP            types.String `tfsdk:"ip"`
	ID            types.Int64  `tfsdk:"id"`
	Cid           types.Int64  `tfsdk:"cid"`
	Event         types.String `tfsdk:"event"`
	Enable        types.Bool   `tfsdk:"enable"`
	Name          types.String `tfsdk:"name"`
	SslCa         types.String `tfsdk:"ssl_ca"`
	Urls          types.List   `tfsdk:"urls"`
	Condition     types.String `tfsdk:"condition"`
	RepeatPeriod  types.Int64  `tfsdk:"repeat_period"`
	ActiveBetween types.List   `tfsdk:"active_between"`
}

// --- wire types (https://shelly-api-docs.shelly.cloud/gen2/ComponentsAndServices/Webhook) ---

type webhookHook struct {
	ID            int      `json:"id"`
	Cid           int      `json:"cid"`
	Enable        bool     `json:"enable"`
	Event         string   `json:"event"`
	Name          *string  `json:"name"`
	SslCa         *string  `json:"ssl_ca"`
	Urls          []string `json:"urls"`
	Condition     *string  `json:"condition"`
	RepeatPeriod  float64  `json:"repeat_period"`
	ActiveBetween []string `json:"active_between,omitempty"`
}

type webhookListResponse struct {
	Hooks []webhookHook `json:"hooks"`
	Rev   int           `json:"rev"`
}
type webhookListRequest struct{}

func (r *webhookListRequest) Method() string   { return "Webhook.List" }
func (r *webhookListRequest) NewResponse() any { return &webhookListResponse{} }

type webhookCreateRequest struct {
	Event         string   `json:"event"`
	Cid           int      `json:"cid"`
	Urls          []string `json:"urls"`
	Enable        *bool    `json:"enable,omitempty"`
	Name          *string  `json:"name,omitempty"`
	SslCa         *string  `json:"ssl_ca,omitempty"`
	Condition     *string  `json:"condition,omitempty"`
	RepeatPeriod  *float64 `json:"repeat_period,omitempty"`
	ActiveBetween []string `json:"active_between,omitempty"`
}
type webhookCreateResponse struct {
	ID  int `json:"id"`
	Rev int `json:"rev"`
}

func (r *webhookCreateRequest) Method() string   { return "Webhook.Create" }
func (r *webhookCreateRequest) NewResponse() any { return &webhookCreateResponse{} }

type webhookUpdateRequest struct {
	ID            int      `json:"id"`
	Event         *string  `json:"event,omitempty"`
	Cid           *int     `json:"cid,omitempty"`
	Enable        *bool    `json:"enable,omitempty"`
	Name          *string  `json:"name,omitempty"`
	SslCa         *string  `json:"ssl_ca,omitempty"`
	Urls          []string `json:"urls,omitempty"`
	Condition     *string  `json:"condition,omitempty"`
	RepeatPeriod  *float64 `json:"repeat_period,omitempty"`
	ActiveBetween []string `json:"active_between,omitempty"`
}
type webhookUpdateResponse struct {
	Rev int `json:"rev"`
}

func (r *webhookUpdateRequest) Method() string   { return "Webhook.Update" }
func (r *webhookUpdateRequest) NewResponse() any { return &webhookUpdateResponse{} }

type webhookDeleteRequest struct {
	ID int `json:"id"`
}
type webhookDeleteResponse struct {
	Rev int `json:"rev"`
}

func (r *webhookDeleteRequest) Method() string   { return "Webhook.Delete" }
func (r *webhookDeleteRequest) NewResponse() any { return &webhookDeleteResponse{} }

// --- resource ---

func (r *webhookResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook"
}

func (r *webhookResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A device-native Webhook (Settings -> Actions in the app): fires an HTTP request to `urls` when `event` occurs on component `cid`. Unlike the generated `shelly_*_config` resources this is a true CRUD object - the device allocates `id` on create and forgets the hook on delete.",
		Attributes: map[string]schema.Attribute{
			"ip": schema.StringAttribute{Required: true, MarkdownDescription: "The IP address of the Shelly device."},
			"id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Webhook id, allocated by the device on create.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"cid":   schema.Int64Attribute{Required: true, MarkdownDescription: "Component instance id the event applies to, e.g. input:0 is cid=0."},
			"event": schema.StringAttribute{Required: true, MarkdownDescription: "Triggering event, e.g. \"input.toggle_on\". See Webhook.ListSupported on the device for the valid set per component type."},
			"enable": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Whether the hook is active.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "User-defined label, shown in the app's Actions list.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"ssl_ca": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "TLS validation mode for https urls: null uses the built-in CA, \"user_ca.pem\" a custom one, \"*\" disables validation.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"urls": schema.ListAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "HTTP endpoints invoked when the event fires.",
			},
			"condition": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Boolean expression gating whether the hook actually fires.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"repeat_period": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Minimum seconds between invocations; 0 means no rate limit.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"active_between": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "[\"HH:MM\", \"HH:MM\"] window the hook is active in, device-local time. Empty means always active.",
				PlanModifiers:       []planmodifier.List{listplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func webhookClient(ip string) *resty.Client {
	client := resty.New()
	client.SetBaseURL("http://" + ip)
	return client
}

// findHook reads the device's full hook list and returns the one matching id, or nil if gone.
func findHook(client *resty.Client, id int) (*webhookHook, error) {
	var listResp webhookListResponse
	if _, err := rpc.Do(client, &webhookListRequest{}, &listResp); err != nil {
		return nil, err
	}
	for i := range listResp.Hooks {
		if listResp.Hooks[i].ID == id {
			return &listResp.Hooks[i], nil
		}
	}
	return nil, nil
}

func (m *webhookResourceModel) fromHook(ctx context.Context, h *webhookHook) {
	m.ID = types.Int64Value(int64(h.ID))
	m.Cid = types.Int64Value(int64(h.Cid))
	m.Event = types.StringValue(h.Event)
	m.Enable = types.BoolValue(h.Enable)
	if h.Name != nil {
		m.Name = types.StringValue(*h.Name)
	} else {
		m.Name = types.StringNull()
	}
	if h.SslCa != nil {
		m.SslCa = types.StringValue(*h.SslCa)
	} else {
		m.SslCa = types.StringNull()
	}
	urls, _ := types.ListValueFrom(ctx, types.StringType, h.Urls)
	m.Urls = urls
	if h.Condition != nil {
		m.Condition = types.StringValue(*h.Condition)
	} else {
		m.Condition = types.StringNull()
	}
	m.RepeatPeriod = types.Int64Value(int64(h.RepeatPeriod))
	activeBetween, _ := types.ListValueFrom(ctx, types.StringType, h.ActiveBetween)
	m.ActiveBetween = activeBetween
}

func (r *webhookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan webhookResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var urls []string
	resp.Diagnostics.Append(plan.Urls.ElementsAs(ctx, &urls, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	create := &webhookCreateRequest{
		Event: plan.Event.ValueString(),
		Cid:   int(plan.Cid.ValueInt64()),
		Urls:  urls,
	}
	if !plan.Enable.IsNull() && !plan.Enable.IsUnknown() {
		v := plan.Enable.ValueBool()
		create.Enable = &v
	}
	if !plan.Name.IsNull() && !plan.Name.IsUnknown() {
		v := plan.Name.ValueString()
		create.Name = &v
	}
	if !plan.SslCa.IsNull() && !plan.SslCa.IsUnknown() {
		v := plan.SslCa.ValueString()
		create.SslCa = &v
	}
	if !plan.Condition.IsNull() && !plan.Condition.IsUnknown() {
		v := plan.Condition.ValueString()
		create.Condition = &v
	}
	if !plan.RepeatPeriod.IsNull() && !plan.RepeatPeriod.IsUnknown() {
		v := float64(plan.RepeatPeriod.ValueInt64())
		create.RepeatPeriod = &v
	}
	if !plan.ActiveBetween.IsNull() && !plan.ActiveBetween.IsUnknown() {
		var ab []string
		resp.Diagnostics.Append(plan.ActiveBetween.ElementsAs(ctx, &ab, false)...)
		create.ActiveBetween = ab
	}
	if resp.Diagnostics.HasError() {
		return
	}

	client := webhookClient(plan.IP.ValueString())
	defer client.Close()

	var createResp webhookCreateResponse
	if _, err := rpc.Do(client, create, &createResp); err != nil {
		resp.Diagnostics.AddError("Failed to create webhook", err.Error())
		return
	}

	hook, err := findHook(client, createResp.ID)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read webhook after create", err.Error())
		return
	}
	if hook == nil {
		resp.Diagnostics.AddError("Webhook not found after create", fmt.Sprintf("Webhook.Create returned id %d but it isn't in Webhook.List", createResp.ID))
		return
	}
	plan.fromHook(ctx, hook)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *webhookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state webhookResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := webhookClient(state.IP.ValueString())
	defer client.Close()

	hook, err := findHook(client, int(state.ID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Failed to read webhook", err.Error())
		return
	}
	if hook == nil {
		// Deleted out of band (device UI/app, factory reset, etc.).
		resp.State.RemoveResource(ctx)
		return
	}
	state.fromHook(ctx, hook)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *webhookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan webhookResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state webhookResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var urls []string
	resp.Diagnostics.Append(plan.Urls.ElementsAs(ctx, &urls, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	event := plan.Event.ValueString()
	cid := int(plan.Cid.ValueInt64())
	update := &webhookUpdateRequest{
		ID:    int(state.ID.ValueInt64()),
		Event: &event,
		Cid:   &cid,
		Urls:  urls,
	}
	if !plan.Enable.IsNull() && !plan.Enable.IsUnknown() {
		v := plan.Enable.ValueBool()
		update.Enable = &v
	}
	if !plan.Name.IsNull() && !plan.Name.IsUnknown() {
		v := plan.Name.ValueString()
		update.Name = &v
	}
	if !plan.SslCa.IsNull() && !plan.SslCa.IsUnknown() {
		v := plan.SslCa.ValueString()
		update.SslCa = &v
	}
	if !plan.Condition.IsNull() && !plan.Condition.IsUnknown() {
		v := plan.Condition.ValueString()
		update.Condition = &v
	}
	if !plan.RepeatPeriod.IsNull() && !plan.RepeatPeriod.IsUnknown() {
		v := float64(plan.RepeatPeriod.ValueInt64())
		update.RepeatPeriod = &v
	}
	if !plan.ActiveBetween.IsNull() && !plan.ActiveBetween.IsUnknown() {
		var ab []string
		resp.Diagnostics.Append(plan.ActiveBetween.ElementsAs(ctx, &ab, false)...)
		update.ActiveBetween = ab
	}
	if resp.Diagnostics.HasError() {
		return
	}

	client := webhookClient(plan.IP.ValueString())
	defer client.Close()

	var updateResp webhookUpdateResponse
	if _, err := rpc.Do(client, update, &updateResp); err != nil {
		resp.Diagnostics.AddError("Failed to update webhook", err.Error())
		return
	}

	hook, err := findHook(client, int(state.ID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Failed to read webhook after update", err.Error())
		return
	}
	if hook == nil {
		resp.Diagnostics.AddError("Webhook not found after update", fmt.Sprintf("id %d is gone from Webhook.List right after updating it", state.ID.ValueInt64()))
		return
	}
	plan.fromHook(ctx, hook)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *webhookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state webhookResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := webhookClient(state.IP.ValueString())
	defer client.Close()

	var deleteResp webhookDeleteResponse
	if _, err := rpc.Do(client, &webhookDeleteRequest{ID: int(state.ID.ValueInt64())}, &deleteResp); err != nil {
		resp.Diagnostics.AddError("Failed to delete webhook", err.Error())
		return
	}
}

func (r *webhookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
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

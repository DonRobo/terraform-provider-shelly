package provider

import (
	"context"
	"encoding/json"
	"fmt"

	shelly "github.com/DonRobo/shelly-go"
	"github.com/DonRobo/shelly-go/rpc"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"resty.dev/v3"
)

var (
	_ resource.Resource              = &authResource{}
	_ resource.ResourceWithConfigure = &authResource{}
)

func NewAuthResource() resource.Resource { return &authResource{} }

type authResource struct {
	provider *providerData
}

type authResourceModel struct {
	IP       types.String `tfsdk:"ip"`
	Password types.String `tfsdk:"password"`
	DeviceID types.String `tfsdk:"device_id"`
}

// setAuthRequest is Shelly.SetAuth. HA1 is a pointer without omitempty: a nil
// HA1 is sent as JSON null, which turns authentication off.
type setAuthRequest struct {
	User  string  `json:"user"`
	Realm string  `json:"realm"`
	HA1   *string `json:"ha1"`
}

func (r *setAuthRequest) Method() string   { return "Shelly.SetAuth" }
func (r *setAuthRequest) NewResponse() any { return &json.RawMessage{} }

func setAuth(client *resty.Client, deviceID string, ha1 *string) error {
	req := &setAuthRequest{User: shellyUser, Realm: deviceID, HA1: ha1}
	_, err := rpc.Do(client, req, req.NewResponse())
	return err
}

func (r *authResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_auth"
}

func (r *authResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Sets the password of a Shelly device (HTTP digest authentication, user `admin`). " +
			"When you delete this resource, the provider turns authentication off on the device. " +
			"To manage other resources on a device with a password, set the provider `password` or the resource `device_password` to the same value.\n\n" +
			"To change the password, change it here and in the provider (or `device_password`) together, then run `terraform apply -refresh=false` one time. " +
			"A refresh before the apply fails, because the device still has the old password.",
		Attributes: map[string]schema.Attribute{
			"ip": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The IP address of the Shelly device.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"password": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "The device password. Terraform keeps it in the state as a sensitive value.",
			},
			"device_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The device ID. Shelly uses it as the digest realm.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *authResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *providerData, got %T", req.ProviderData))
		return
	}
	r.provider = pd
}

// Create authenticates with the provider password, which is necessary when the
// device already has a password.
func (r *authResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan authResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	client := r.provider.client(plan.IP.ValueString(), types.StringNull())
	defer client.Close()

	info, _, err := (&shelly.ShellyGetDeviceInfoRequest{}).Do(client)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read device info", err.Error())
		return
	}
	ha1 := authHA1(info.ID, plan.Password.ValueString())
	if err := setAuth(client, info.ID, &ha1); err != nil {
		resp.Diagnostics.AddError("Failed to set device password", err.Error())
		return
	}
	plan.DeviceID = types.StringValue(info.ID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read removes the resource when someone turned authentication off on the device.
func (r *authResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state authResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	client := r.provider.client(state.IP.ValueString(), state.Password)
	defer client.Close()

	info, _, err := (&shelly.ShellyGetDeviceInfoRequest{}).Do(client)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read device info", err.Error())
		return
	}
	if info.AuthEn == nil || !*info.AuthEn {
		resp.State.RemoveResource(ctx)
		return
	}
	state.DeviceID = types.StringValue(info.ID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update authenticates with the old password from the state, then sets the new one.
func (r *authResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state authResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	client := r.provider.client(state.IP.ValueString(), state.Password)
	defer client.Close()

	ha1 := authHA1(state.DeviceID.ValueString(), plan.Password.ValueString())
	if err := setAuth(client, state.DeviceID.ValueString(), &ha1); err != nil {
		resp.Diagnostics.AddError("Failed to change device password", err.Error())
		return
	}
	plan.DeviceID = state.DeviceID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete turns authentication off on the device.
func (r *authResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state authResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	client := r.provider.client(state.IP.ValueString(), state.Password)
	defer client.Close()

	if err := setAuth(client, state.DeviceID.ValueString(), nil); err != nil {
		resp.Diagnostics.AddError("Failed to turn off device authentication", err.Error())
	}
}

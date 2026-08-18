package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/stretchr/testify/require"
)

func TestProviderSchema(t *testing.T) {
	p := New("test")()
	ctx := context.Background()
	var req provider.SchemaRequest
	var resp provider.SchemaResponse
	p.Schema(ctx, req, &resp)
	require.Empty(t, resp.Diagnostics.Errors())
	require.NotNil(t, resp.Schema)
}

func TestSysConfigResourceSchema(t *testing.T) {
	res := NewSysConfigResource()
	ctx := context.Background()
	var req resource.SchemaRequest
	var resp resource.SchemaResponse
	res.Schema(ctx, req, &resp)
	require.Empty(t, resp.Diagnostics.Errors())
	require.NotNil(t, resp.Schema)
	reqAttrs := resp.Schema.Attributes
	require.Contains(t, reqAttrs, "ip")
	// Sys config is nested: name/eco_mode live under the "device" object.
	require.Contains(t, reqAttrs, "device")
	device, ok := reqAttrs["device"].(schema.SingleNestedAttribute)
	require.True(t, ok)
	require.Contains(t, device.Attributes, "name")
	require.Contains(t, device.Attributes, "eco_mode")
}

func TestInputConfigResourceSchema(t *testing.T) {
	res := NewInputConfigResource()
	ctx := context.Background()
	var req resource.SchemaRequest
	var resp resource.SchemaResponse
	res.Schema(ctx, req, &resp)
	require.Empty(t, resp.Diagnostics.Errors())
	require.NotNil(t, resp.Schema)
	reqAttrs := resp.Schema.Attributes
	require.Contains(t, reqAttrs, "ip")
	require.Contains(t, reqAttrs, "id")
}

func TestSwitchConfigResourceSchema(t *testing.T) {
	res := NewSwitchConfigResource()
	ctx := context.Background()
	var req resource.SchemaRequest
	var resp resource.SchemaResponse
	res.Schema(ctx, req, &resp)
	require.Empty(t, resp.Diagnostics.Errors())
	require.NotNil(t, resp.Schema)
	reqAttrs := resp.Schema.Attributes
	require.Contains(t, reqAttrs, "ip")
	require.Contains(t, reqAttrs, "id")
}

func TestWebhookResourceSchema(t *testing.T) {
	res := NewWebhookResource()
	ctx := context.Background()
	var req resource.SchemaRequest
	var resp resource.SchemaResponse
	res.Schema(ctx, req, &resp)
	require.Empty(t, resp.Diagnostics.Errors())
	require.NotNil(t, resp.Schema)
	reqAttrs := resp.Schema.Attributes
	require.Contains(t, reqAttrs, "ip")
	require.Contains(t, reqAttrs, "id")
	require.Contains(t, reqAttrs, "cid")
	require.Contains(t, reqAttrs, "event")
	require.Contains(t, reqAttrs, "urls")
	// Unlike the generated *_config resources, id is not a required input -
	// the device allocates it on create.
	require.False(t, reqAttrs["id"].IsRequired())
	require.True(t, reqAttrs["id"].IsComputed())
	require.True(t, reqAttrs["urls"].IsRequired())
}

func TestWebhookResourceRegistered(t *testing.T) {
	p := &ShellyProvider{}
	ctx := context.Background()
	found := false
	for _, f := range p.Resources(ctx) {
		if _, ok := f().(*webhookResource); ok {
			found = true
			break
		}
	}
	require.True(t, found, "NewWebhookResource must be registered in ShellyProvider.Resources")
}

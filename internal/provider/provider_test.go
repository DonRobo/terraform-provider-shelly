package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
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
	require.Contains(t, reqAttrs, "name")
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

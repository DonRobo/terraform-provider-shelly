// Command gen emits Terraform resources from the shelly-go IR.
//
// For every Shelly component that exposes SetConfig it writes a
// shelly_<component>_config resource covering the component's configuration:
// scalar fields become typed attributes and nested objects become
// SingleNestedAttribute blocks mirroring the library's nested config structs.
// Array fields are not yet represented (a follow-up).
//
// The whole client is generated, so there is no hand-written code to skip: a
// hand-written resource defining the same New<Prefix>ConfigResource would
// collide at build time, which is the intended signal to delete it.
//
//	go run ./internal/provider/gen   (or: go generate ./...)
package main

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/DonRobo/shelly-go/gen"
)

// floatLit renders a documented numeric bound as a minimal Go float literal
// (no scientific notation), e.g. 0.5, 30, 2147483647.
func floatLit(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

const outDir = "internal/provider"

func main() {
	spec, err := gen.Load()
	check(err)

	check(removeGenerated(outDir))

	var generated []string
	for _, c := range spec.Components {
		if c.Name == "Shelly" {
			continue // the Shelly service is the aggregate, not a config component
		}
		if !c.HasSetConfig {
			continue
		}
		root := buildTree(c)
		if len(root.order) == 0 {
			continue // nothing representable (only id/ip, arrays, or opaque objects)
		}
		src, err := emitResource(c, root)
		if err != nil {
			check(fmt.Errorf("emit %s: %w", c.Name, err))
		}
		dst := filepath.Join(outDir, strings.ToLower(c.Name)+"_config_resource_gen.go")
		check(os.WriteFile(dst, src, 0o644))
		generated = append(generated, c.Prefix())
	}

	reg, err := emitRegistration(generated)
	check(err)
	check(os.WriteFile(filepath.Join(outDir, "resources_gen.go"), reg, 0o644))
	fmt.Fprintf(os.Stderr, "generated %d config resources: %s\n", len(generated), strings.Join(generated, ", "))
}

// --- config field tree -------------------------------------------------------

// tnode is a node in the config tree: a leaf carries a scalar field; an internal
// node is a nested object. Only scalar leaves (and their ancestors) are kept, so
// every internal node is guaranteed to have a representable descendant.
type tnode struct {
	seg      string
	field    *gen.Field // non-nil for leaves
	order    []string
	children map[string]*tnode
}

func newTnode(seg string) *tnode {
	return &tnode{seg: seg, children: map[string]*tnode{}}
}

func (n *tnode) child(seg string) *tnode {
	c, ok := n.children[seg]
	if !ok {
		c = newTnode(seg)
		n.children[seg] = c
		n.order = append(n.order, seg)
	}
	return c
}

func (n *tnode) leaf() bool { return n.field != nil }

// buildTree builds the config tree from a component's fields, keeping the
// representable leaves (scalars and arrays of scalars) and dropping the identity
// keys and opaque objects. Intermediate objects appear via the dotted key
// segments.
func buildTree(c *gen.Component) *tnode {
	root := newTnode("")
	for _, f := range c.Fields {
		if f.Key == "id" || f.Key == "ip" {
			continue
		}
		if !representable(f) {
			continue // opaque object / typeless array — not representable
		}
		cur := root
		segs := strings.Split(f.Key, ".")
		for i, s := range segs {
			cur = cur.child(s)
			if i == len(segs)-1 {
				cur.field = f
			}
		}
	}
	return root
}

func scalar(t string) bool {
	switch t {
	case "string", "number", "integer", "boolean":
		return true
	}
	return false
}

// representable reports whether a leaf field can be exposed as a Terraform
// attribute: a scalar, or an array whose element type the docs name.
func representable(f *gen.Field) bool {
	if scalar(f.Type) {
		return true
	}
	_, _, ok := arrayInfo(f.Elem)
	return f.Type == "array" && ok
}

// arrayInfo maps an array element base type to its Terraform element type and the
// library's Go slice type. ok is false for arrays whose element type the docs
// don't specify (the library leaves those as json.RawMessage; we skip them).
func arrayInfo(elem string) (tfElemType, goSlice string, ok bool) {
	switch elem {
	case "number":
		return "types.Float64Type", "[]float64", true
	case "integer":
		return "types.Int64Type", "[]int", true
	case "string":
		return "types.StringType", "[]string", true
	case "boolean":
		return "types.BoolType", "[]bool", true
	}
	return "", "", false
}

// readOnly reports whether a field is documented as read-only (its description
// starts with "Read-only"). Such fields are Computed-only: surfaced in state but
// never sent to SetConfig. Fields that merely mention read-only mid-sentence with
// caveats (e.g. enhanced_security) stay settable.
func readOnly(f *gen.Field) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(f.Description)), "read-only")
}

// modelFieldType is the Terraform model field type for a leaf.
func modelFieldType(f *gen.Field) string {
	if f.Type == "array" {
		return "types.List"
	}
	return info(f.Type).Model
}

// --- emission ----------------------------------------------------------------

func emitResource(c *gen.Component, root *tnode) ([]byte, error) {
	prefix := c.Prefix()
	lower := strings.ToLower(c.Name)
	typeName := lower + "ConfigResource"

	imp := newImports(
		"context",
		"github.com/DonRobo/shelly-go/components",
		"github.com/hashicorp/terraform-plugin-framework/diag",
		"github.com/hashicorp/terraform-plugin-framework/path",
		"github.com/hashicorp/terraform-plugin-framework/resource",
		"github.com/hashicorp/terraform-plugin-framework/resource/schema",
		"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier",
		"github.com/hashicorp/terraform-plugin-framework/types",
		"resty.dev/v3",
	)
	if c.Keyed {
		imp.add("fmt", "strconv", "strings")
	}

	var b strings.Builder
	b.WriteString("// Code generated by internal/provider/gen from the Shelly API docs. DO NOT EDIT.\n\n")
	b.WriteString("package provider\n\n@IMPORTS@\n")

	fmt.Fprintf(&b, "var (\n\t_ resource.Resource = &%s{}\n\t_ resource.ResourceWithImportState = &%s{}\n)\n\n", typeName, typeName)
	fmt.Fprintf(&b, "func New%sConfigResource() resource.Resource { return &%s{} }\n\n", prefix, typeName)
	fmt.Fprintf(&b, "type %s struct{}\n\n", typeName)

	emitModels(&b, lower, c, root)

	fmt.Fprintf(&b, "func (r *%s) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {\n\tresp.TypeName = req.ProviderTypeName + %q\n}\n\n", typeName, "_"+lower+"_config")

	emitSchema(&b, typeName, c, root, imp)
	emitRead(&b, typeName, lower, c, root)
	emitWrite(&b, typeName, lower, c, root)
	fmt.Fprintf(&b, "func (r *%s) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {\n\tresp.State.RemoveResource(ctx)\n}\n\n", typeName)
	emitImportState(&b, typeName, c)

	src := strings.Replace(b.String(), "@IMPORTS@", imp.block(), 1)
	out, err := format.Source([]byte(src))
	if err != nil {
		return nil, fmt.Errorf("%w\n--- source ---\n%s", err, src)
	}
	return out, nil
}

// modelType returns the Go model type for the node at path. The root is the
// resource model; nested objects get <lower>Config<Seg...>Model, mirroring the
// library's <Prefix>Config<Seg...> nesting.
func modelType(lower string, path []string) string {
	if len(path) == 0 {
		return lower + "ConfigResourceModel"
	}
	return lower + "Config" + goNamePath(path) + "Model"
}

// libType returns the library config struct type for the object node at path,
// e.g. SysConfigDevice for prefix "Sys" and path ["device"].
func libType(prefix string, path []string) string {
	return prefix + "Config" + goNamePath(path)
}

// attrName is the Terraform attribute name for a JSON key segment. Terraform
// requires lowercase [a-z0-9_]; a few Shelly config keys carry an uppercase unit
// suffix (report_thr_C, offset_C), so lower-case the segment. The Go field name
// (GoName) is unaffected, so the mapping to the library struct still lines up.
func attrName(seg string) string { return strings.ToLower(seg) }

func goNamePath(path []string) string {
	var b strings.Builder
	for _, s := range path {
		b.WriteString(gen.GoName(s))
	}
	return b.String()
}

// child returns path with seg appended, copying so callers never share backing.
func childPath(path []string, seg string) []string {
	return append(append([]string{}, path...), seg)
}

// emitModels writes the nested object model structs (deepest first) and then the
// root resource model.
func emitModels(b *strings.Builder, lower string, c *gen.Component, root *tnode) {
	emitNestedModels(b, lower, nil, root)

	fmt.Fprintf(b, "type %s struct {\n\tIP types.String `tfsdk:\"ip\"`\n", modelType(lower, nil))
	if c.Keyed {
		b.WriteString("\tID types.Int64 `tfsdk:\"id\"`\n")
	}
	emitModelFields(b, lower, nil, root)
	b.WriteString("}\n\n")
}

func emitNestedModels(b *strings.Builder, lower string, path []string, n *tnode) {
	for _, seg := range n.order {
		c := n.children[seg]
		if c.leaf() {
			continue
		}
		sub := childPath(path, seg)
		emitNestedModels(b, lower, sub, c)
		fmt.Fprintf(b, "type %s struct {\n", modelType(lower, sub))
		emitModelFields(b, lower, sub, c)
		b.WriteString("}\n\n")
	}
}

func emitModelFields(b *strings.Builder, lower string, path []string, n *tnode) {
	for _, seg := range n.order {
		c := n.children[seg]
		g := gen.GoName(seg)
		if c.leaf() {
			fmt.Fprintf(b, "\t%s %s `tfsdk:%q`\n", g, modelFieldType(c.field), attrName(seg))
			continue
		}
		sub := childPath(path, seg)
		fmt.Fprintf(b, "\t%s *%s `tfsdk:%q`\n", g, modelType(lower, sub), attrName(seg))
	}
}

func emitSchema(b *strings.Builder, typeName string, c *gen.Component, root *tnode, imp *imports) {
	fmt.Fprintf(b, "func (r *%s) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {\n", typeName)
	b.WriteString("\tresp.Schema = schema.Schema{\n\t\tAttributes: map[string]schema.Attribute{\n")
	b.WriteString("\t\t\t\"ip\": schema.StringAttribute{Required: true, MarkdownDescription: \"The IP address of the Shelly device.\"},\n")
	if c.Keyed {
		fmt.Fprintf(b, "\t\t\t\"id\": schema.Int64Attribute{Required: true, MarkdownDescription: \"The ID of the %s component instance.\"},\n", c.Name)
	}
	emitSchemaAttrs(b, root, imp)
	b.WriteString("\t\t},\n\t}\n}\n\n")
}

func emitSchemaAttrs(b *strings.Builder, n *tnode, imp *imports) {
	for _, seg := range n.order {
		c := n.children[seg]
		if !c.leaf() {
			imp.add("github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier")
			fmt.Fprintf(b, "%q: schema.SingleNestedAttribute{\nOptional: true,\nComputed: true,\nPlanModifiers: []planmodifier.Object{objectplanmodifier.UseStateForUnknown()},\nAttributes: map[string]schema.Attribute{\n", attrName(seg))
			emitSchemaAttrs(b, c, imp)
			b.WriteString("},\n},\n")
			continue
		}
		f := c.field
		ro := readOnly(f)

		// availability is the Optional/Computed line: settable fields are both;
		// read-only fields are Computed-only so plans never try to set them.
		availability := "Optional: true,\nComputed: true,\n"
		if ro {
			availability = "Computed: true,\n"
		}

		if f.Type == "array" {
			tfElem, _, _ := arrayInfo(f.Elem)
			imp.add("github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier")
			fmt.Fprintf(b, "%q: schema.ListAttribute{\nElementType: %s,\n%sMarkdownDescription: %q,\n", attrName(seg), tfElem, availability, f.Description)
			b.WriteString("PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()},\n},\n")
			continue
		}

		ti := info(f.Type)
		fmt.Fprintf(b, "%q: %s{\n%sMarkdownDescription: %q,\n", attrName(seg), ti.Attr, availability, f.Description)
		if !ro && f.Type == "string" && len(f.Enum) > 0 {
			imp.add("github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator",
				"github.com/hashicorp/terraform-plugin-framework/schema/validator")
			quoted := make([]string, len(f.Enum))
			for i, e := range f.Enum {
				quoted[i] = fmt.Sprintf("%q", e)
			}
			fmt.Fprintf(b, "Validators: []validator.String{stringvalidator.OneOf(%s)},\n", strings.Join(quoted, ", "))
		}
		if !ro && (f.Type == "number" || f.Type == "integer") && f.Min != nil && f.Max != nil {
			imp.add("github.com/hashicorp/terraform-plugin-framework/schema/validator")
			if f.Type == "integer" {
				imp.add("github.com/hashicorp/terraform-plugin-framework-validators/int64validator")
				fmt.Fprintf(b, "Validators: []validator.Int64{int64validator.Between(%d, %d)},\n", int64(*f.Min), int64(*f.Max))
			} else {
				imp.add("github.com/hashicorp/terraform-plugin-framework-validators/float64validator")
				fmt.Fprintf(b, "Validators: []validator.Float64{float64validator.Between(%s, %s)},\n", floatLit(*f.Min), floatLit(*f.Max))
			}
		}
		imp.add("github.com/hashicorp/terraform-plugin-framework/resource/schema/" + ti.PlanPkg)
		fmt.Fprintf(b, "PlanModifiers: []planmodifier.%s{%s.UseStateForUnknown()},\n},\n", ti.PlanKnd, ti.PlanPkg)
	}
}

func emitRead(b *strings.Builder, typeName, lower string, c *gen.Component, root *tnode) {
	model := modelType(lower, nil)

	// get reads the live config into m. It is shared by Read and by Create/Update,
	// which read back after applying so Computed values (read-only fields, server
	// defaults) are known in state.
	fmt.Fprintf(b, "func (r *%s) get(ctx context.Context, m *%s, diags *diag.Diagnostics) {\n", typeName, model)
	b.WriteString("\tclient := resty.New()\n\tdefer client.Close()\n\tclient.SetBaseURL(\"http://\" + m.IP.ValueString())\n")
	if c.Keyed {
		fmt.Fprintf(b, "\tgot, _, err := (&components.%sGetConfigRequest{ID: int(m.ID.ValueInt64())}).Do(client)\n", c.Prefix())
	} else {
		fmt.Fprintf(b, "\tgot, _, err := (&components.%sGetConfigRequest{}).Do(client)\n", c.Prefix())
	}
	b.WriteString("\tif err != nil {\n\t\tdiags.AddError(\"Failed to read config\", err.Error())\n\t\treturn\n\t}\n")
	emitReadNode(b, lower, "got", "m", nil, root)
	b.WriteString("}\n\n")

	fmt.Fprintf(b, "func (r *%s) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {\n\tvar state %s\n", typeName, model)
	b.WriteString("\tresp.Diagnostics.Append(req.State.Get(ctx, &state)...)\n\tif resp.Diagnostics.HasError() {\n\t\treturn\n\t}\n")
	b.WriteString("\tr.get(ctx, &state, &resp.Diagnostics)\n\tif resp.Diagnostics.HasError() {\n\t\treturn\n\t}\n")
	b.WriteString("\tresp.Diagnostics.Append(resp.State.Set(ctx, &state)...)\n}\n\n")
}

func emitReadNode(b *strings.Builder, lower, got, state string, path []string, n *tnode) {
	for _, seg := range n.order {
		c := n.children[seg]
		g := gen.GoName(seg)
		if c.leaf() {
			f := c.field
			if f.Type == "array" {
				tfElem, _, _ := arrayInfo(f.Elem)
				fmt.Fprintf(b, "\tif %s.%s != nil {\n\t\tl, d := types.ListValueFrom(ctx, %s, %s.%s)\n\t\tdiags.Append(d...)\n\t\t%s.%s = l\n\t}\n", got, g, tfElem, got, g, state, g)
			} else {
				fmt.Fprintf(b, "\tif %s.%s != nil {\n\t\t%s.%s = %s\n\t}\n", got, g, state, g, readConv(f.Type, got+"."+g))
			}
			continue
		}
		sub := childPath(path, seg)
		cg, cs := got+"."+g, state+"."+g
		fmt.Fprintf(b, "\tif %s != nil {\n", cg)
		fmt.Fprintf(b, "\t\tif %s == nil {\n\t\t\t%s = &%s{}\n\t\t}\n", cs, cs, modelType(lower, sub))
		emitReadNode(b, lower, cg, cs, sub, c)
		b.WriteString("\t}\n")
	}
}

func emitWrite(b *strings.Builder, typeName, lower string, c *gen.Component, root *tnode) {
	model := modelType(lower, nil)

	fmt.Fprintf(b, "func (r *%s) apply(ctx context.Context, plan %s, diags *diag.Diagnostics) {\n\tvar cfg components.%sConfig\n", typeName, model, c.Prefix())
	if c.Keyed {
		b.WriteString("\tcfg.ID = int(plan.ID.ValueInt64())\n")
	}
	emitWriteNode(b, c.Prefix(), "cfg", "plan", nil, root)
	b.WriteString("\tclient := resty.New()\n\tdefer client.Close()\n\tclient.SetBaseURL(\"http://\" + plan.IP.ValueString())\n")
	if c.Keyed {
		fmt.Fprintf(b, "\tif _, _, err := (&components.%sSetConfigRequest{ID: int(plan.ID.ValueInt64()), Config: cfg}).Do(client); err != nil {\n", c.Prefix())
	} else {
		fmt.Fprintf(b, "\tif _, _, err := (&components.%sSetConfigRequest{Config: cfg}).Do(client); err != nil {\n", c.Prefix())
	}
	b.WriteString("\t\tdiags.AddError(\"Failed to set config\", err.Error())\n\t}\n}\n\n")

	for _, op := range []string{"Create", "Update"} {
		fmt.Fprintf(b, "func (r *%s) %s(ctx context.Context, req resource.%sRequest, resp *resource.%sResponse) {\n\tvar plan %s\n", typeName, op, op, op, model)
		b.WriteString("\tresp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)\n\tif resp.Diagnostics.HasError() {\n\t\treturn\n\t}\n")
		b.WriteString("\tr.apply(ctx, plan, &resp.Diagnostics)\n\tif resp.Diagnostics.HasError() {\n\t\treturn\n\t}\n")
		// Read back so Computed values (read-only fields, server defaults) are known.
		b.WriteString("\tr.get(ctx, &plan, &resp.Diagnostics)\n\tif resp.Diagnostics.HasError() {\n\t\treturn\n\t}\n")
		b.WriteString("\tresp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)\n}\n\n")
	}
}

func emitWriteNode(b *strings.Builder, prefix, cfg, plan string, path []string, n *tnode) {
	for _, seg := range n.order {
		c := n.children[seg]
		g := gen.GoName(seg)
		if c.leaf() {
			f := c.field
			if readOnly(f) {
				continue // read-only: surfaced in state, never sent to SetConfig
			}
			if f.Type == "array" {
				_, goSlice, _ := arrayInfo(f.Elem)
				fmt.Fprintf(b, "\tif !%s.%s.IsNull() && !%s.%s.IsUnknown() {\n\t\tvar v %s\n\t\tdiags.Append(%s.%s.ElementsAs(ctx, &v, false)...)\n\t\t%s.%s = v\n\t}\n", plan, g, plan, g, goSlice, plan, g, cfg, g)
				continue
			}
			fmt.Fprintf(b, "\tif !%s.%s.IsNull() && !%s.%s.IsUnknown() {\n\t\t%s\n\t}\n", plan, g, plan, g, setStmt(f.Type, cfg+"."+g, plan+"."+g))
			continue
		}
		sub := childPath(path, seg)
		cc, cp := cfg+"."+g, plan+"."+g
		fmt.Fprintf(b, "\tif %s != nil {\n", cp)
		fmt.Fprintf(b, "\t\t%s = &components.%s{}\n", cc, libType(prefix, sub))
		emitWriteNode(b, prefix, cc, cp, sub, c)
		b.WriteString("\t}\n")
	}
}

func emitImportState(b *strings.Builder, typeName string, c *gen.Component) {
	fmt.Fprintf(b, "func (r *%s) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {\n", typeName)
	if c.Keyed {
		b.WriteString("\tparts := strings.Split(req.ID, \":\")\n\tif len(parts) != 2 {\n\t\tresp.Diagnostics.AddError(\"Invalid import ID\", \"Expected format ip:id\")\n\t\treturn\n\t}\n")
		b.WriteString("\tid, err := strconv.Atoi(parts[1])\n\tif err != nil {\n\t\tresp.Diagnostics.AddError(\"Invalid import ID\", fmt.Sprintf(\"id must be an integer: %v\", err))\n\t\treturn\n\t}\n")
		b.WriteString("\tresp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(\"ip\"), parts[0])...)\n\tresp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(\"id\"), id)...)\n}\n\n")
	} else {
		b.WriteString("\tresource.ImportStatePassthroughID(ctx, path.Root(\"ip\"), req, resp)\n}\n\n")
	}
}

// --- scalar plumbing ---------------------------------------------------------

// typeInfo maps an IR scalar type to its Terraform plumbing.
type typeInfo struct {
	Model   string // types.String
	Attr    string // schema.StringAttribute
	PlanPkg string // stringplanmodifier
	PlanKnd string // String
}

func info(t string) typeInfo {
	switch t {
	case "boolean":
		return typeInfo{"types.Bool", "schema.BoolAttribute", "boolplanmodifier", "Bool"}
	case "number":
		return typeInfo{"types.Float64", "schema.Float64Attribute", "float64planmodifier", "Float64"}
	case "integer":
		return typeInfo{"types.Int64", "schema.Int64Attribute", "int64planmodifier", "Int64"}
	default:
		return typeInfo{"types.String", "schema.StringAttribute", "stringplanmodifier", "String"}
	}
}

// readConv renders the conversion from a library *T field (src) to a TF value.
func readConv(t, src string) string {
	switch t {
	case "boolean":
		return "types.BoolValue(*" + src + ")"
	case "number":
		return "types.Float64Value(*" + src + ")"
	case "integer":
		return "types.Int64Value(int64(*" + src + "))"
	default:
		return "types.StringValue(*" + src + ")"
	}
}

// setStmt renders the statement assigning a TF value (src) into a library *T
// field (dst). Each call lives in its own if-block, so the local v never clashes.
func setStmt(t, dst, src string) string {
	switch t {
	case "boolean":
		return "v := " + src + ".ValueBool(); " + dst + " = &v"
	case "number":
		return "v := " + src + ".ValueFloat64(); " + dst + " = &v"
	case "integer":
		return "v := int(" + src + ".ValueInt64()); " + dst + " = &v"
	default:
		return "v := " + src + ".ValueString(); " + dst + " = &v"
	}
}

func emitRegistration(prefixes []string) ([]byte, error) {
	var b strings.Builder
	b.WriteString("// Code generated by internal/provider/gen. DO NOT EDIT.\n\n")
	b.WriteString("package provider\n\n")
	b.WriteString("import \"github.com/hashicorp/terraform-plugin-framework/resource\"\n\n")
	b.WriteString("// generatedConfigResources are the config resources emitted from the Shelly API docs.\n")
	b.WriteString("func generatedConfigResources() []func() resource.Resource {\n\treturn []func() resource.Resource{\n")
	for _, p := range prefixes {
		fmt.Fprintf(&b, "\t\tNew%sConfigResource,\n", p)
	}
	b.WriteString("\t}\n}\n")
	return format.Source([]byte(b.String()))
}

// --- imports helper ----------------------------------------------------------

type imports struct {
	set map[string]bool
}

func newImports(paths ...string) *imports {
	i := &imports{set: map[string]bool{}}
	i.add(paths...)
	return i
}

func (i *imports) add(paths ...string) {
	for _, p := range paths {
		i.set[p] = true
	}
}

func (i *imports) block() string {
	paths := make([]string, 0, len(i.set))
	for p := range i.set {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var b strings.Builder
	b.WriteString("import (\n")
	for _, p := range paths {
		fmt.Fprintf(&b, "\t%q\n", p)
	}
	b.WriteString(")\n")
	return b.String()
}

// --- misc helpers ------------------------------------------------------------

func removeGenerated(dir string) error {
	for _, pat := range []string{"*_config_resource_gen.go", "resources_gen.go"} {
		matches, err := filepath.Glob(filepath.Join(dir, pat))
		if err != nil {
			return err
		}
		for _, m := range matches {
			if err := os.Remove(m); err != nil {
				return err
			}
		}
	}
	return nil
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

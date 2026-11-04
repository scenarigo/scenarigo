package lsp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"dario.cat/mergo"
	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/scenarigo/scenarigo/internal/filepathutil"
	"github.com/scenarigo/scenarigo/internal/lsp/yamlutil"
	"github.com/scenarigo/scenarigo/protocol/grpc"
	grpcproto "github.com/scenarigo/scenarigo/protocol/grpc/proto"
)

// gRPC support from proto files.
//
// A gRPC step reads proto files when reflection is disabled and its options
// list proto files, the step's options winning over the ones of
// scenarigo.yaml (protocol/grpc/request.go, request_proto_client.go). The
// server applies the same rules, compiles the files with scenarigo's
// compiler, and uses the service descriptors for completion, diagnostics and
// hover. It never uses reflection, which would need a connection. Compiled
// files are cached by import paths, files and modification times, and
// forgotten when a proto file changes on disk; a compile error only means
// no proto support for the step.

// protoCompileTimeout bounds one compilation.
const protoCompileTimeout = 5 * time.Second

type protoCacheEntry struct {
	fds grpcproto.FileDescriptors
	err error
}

// stepProto returns the compiled proto files of a gRPC step, or nil when the
// step does not use proto files or they cannot be compiled.
func (s *Server) stepProto(doc *document, v *yamlView, step ast.Node) grpcproto.FileDescriptors {
	entries := v.entries(step)
	if v.stringValue(v.entryValue(entries, "protocol")) != "grpc" {
		return nil
	}
	opts := &grpc.RequestOptions{}
	if node := v.entryValue(v.entries(v.entryValue(entries, "request")), "options"); node != nil {
		var stepOpts grpc.RequestOptions
		if err := yaml.NodeToValue(node, &stepOpts); err != nil {
			return nil
		}
		if err := mergo.Merge(opts, &stepOpts); err != nil {
			return nil
		}
	}
	if cfg := s.grpcConfigOptions(); cfg != nil && cfg.Request != nil {
		if err := mergo.Merge(opts, cfg.Request, mergo.WithoutDereference); err != nil {
			return nil
		}
	}
	if opts.Reflection.IsEnabled() || opts.Proto == nil || len(opts.Proto.Files) == 0 {
		return nil
	}
	// Paths are relative to the scenario, even those of scenarigo.yaml.
	dir := filepath.Dir(uriToPath(doc.URI))
	imports := make([]string, 0, len(opts.Proto.Imports))
	files := make([]string, 0, len(opts.Proto.Files))
	for _, p := range opts.Proto.Imports {
		imports = append(imports, filepathutil.From(dir, p))
	}
	for _, p := range opts.Proto.Files {
		if len(imports) == 0 {
			p = filepathutil.From(dir, p)
		}
		files = append(files, p)
	}
	for _, p := range slices.Concat(imports, files) {
		if strings.Contains(p, "{{") {
			return nil // evaluated at run time
		}
	}
	return s.compileProtos(imports, files)
}

// grpcConfigOptions returns the gRPC options of scenarigo.yaml.
func (s *Server) grpcConfigOptions() *grpc.Option {
	text, _, ok := s.readConfigText()
	if !ok {
		return nil
	}
	parsed := yamlutil.Parse(text)
	if parsed == nil || parsed.File == nil || len(parsed.File.Docs) == 0 {
		return nil
	}
	v := newYAMLView(parsed.File)
	node := v.entryValue(v.entries(v.entryValue(v.entries(parsed.File.Docs[0].Body), "protocols")), "grpc")
	if node == nil {
		return nil
	}
	var opt grpc.Option
	if err := yaml.NodeToValue(node, &opt); err != nil {
		return nil
	}
	return &opt
}

// compileProtos compiles proto files, or returns the cached result.
func (s *Server) compileProtos(imports, files []string) grpcproto.FileDescriptors {
	key := protoCacheKey(imports, files)
	s.protoMu.Lock()
	defer s.protoMu.Unlock()
	if e, ok := s.protoCache[key]; ok {
		return e.fds
	}
	ctx, cancel := context.WithTimeout(context.Background(), protoCompileTimeout)
	defer cancel()
	fds, err := grpcproto.NewCompiler(imports, grpcproto.WithSourceInfo()).Compile(ctx, files)
	if err != nil {
		s.logger.Printf("compile proto files %v: %v", files, err)
		fds = nil
	}
	if s.protoCache == nil {
		s.protoCache = map[string]*protoCacheEntry{}
	}
	s.protoCache[key] = &protoCacheEntry{fds: fds, err: err}
	return fds
}

// protoCacheKey identifies a compilation by its import paths, its files and
// the modification times of the files.
func protoCacheKey(imports, files []string) string {
	var b strings.Builder
	b.WriteString(strings.Join(imports, "\x00"))
	for _, f := range files {
		b.WriteString("\x01" + f)
		candidates := []string{f}
		if len(imports) > 0 {
			candidates = nil
			for _, imp := range imports {
				candidates = append(candidates, filepath.Join(imp, f))
			}
		}
		for _, c := range candidates {
			if info, err := os.Stat(c); err == nil {
				b.WriteString("@" + info.ModTime().String())
				break
			}
		}
	}
	return b.String()
}

// forgetProtos drops the compiled proto files, after a proto file changed.
func (s *Server) forgetProtos() {
	s.protoMu.Lock()
	defer s.protoMu.Unlock()
	s.protoCache = nil
}

// stepAtLine returns the step of the scenario document at a line that
// contains it.
func stepAtLine(doc *document, v *yamlView, line int) ast.Node {
	if doc.Parsed == nil || doc.Parsed.File == nil {
		return nil
	}
	starts := docStartLines(doc.Parsed.File)
	body := v.entries(doc.Parsed.File.Docs[docIndexAt(starts, line)].Body)
	var found ast.Node
	for _, step := range v.sequence(v.entryValue(body, nsSteps)) {
		if first, _ := nodeLines(step); first >= 0 && first <= line {
			found = step
		}
	}
	return found
}

// protoMethod returns the method a step calls, or nil.
func protoMethod(fds grpcproto.FileDescriptors, v *yamlView, step ast.Node) protoreflect.MethodDescriptor {
	request := v.entries(v.entryValue(v.entries(step), "request"))
	sd, err := fds.ResolveService(protoreflect.FullName(v.stringValue(v.entryValue(request, "service"))))
	if err != nil {
		return nil
	}
	return sd.Methods().ByName(protoreflect.Name(v.stringValue(v.entryValue(request, "method"))))
}

// messageOf returns the message of a step that a path below the step
// starts in: the input of the method under request.message(s), the output
// under expect.message(s). It also returns the rest of the path.
func messageOf(md protoreflect.MethodDescriptor, path []string) (protoreflect.MessageDescriptor, []string) {
	if md == nil || len(path) < 2 {
		return nil, nil
	}
	var msg protoreflect.MessageDescriptor
	switch path[0] {
	case "request":
		msg = md.Input()
	case "expect":
		msg = md.Output()
	default:
		return nil, nil
	}
	switch path[1] {
	case "message":
		return msg, path[2:]
	case "messages":
		rest := path[2:]
		if len(rest) > 0 && isNumeric(rest[0]) {
			rest = rest[1:]
		}
		return msg, rest
	}
	return nil, nil
}

// isNumeric reports whether s is a sequence index.
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// findField finds a field by the names scenarigo accepts: the proto name,
// the text name and the JSON name.
func findField(md protoreflect.MessageDescriptor, name string) protoreflect.FieldDescriptor {
	fields := md.Fields()
	for i := range fields.Len() {
		f := fields.Get(i)
		if string(f.Name()) == name || f.TextName() == name || f.JSONName() == name {
			return f
		}
	}
	return nil
}

// wellKnown reports whether a message has a JSON form of its own
// (google.protobuf.Timestamp is a string, Struct any JSON).
func wellKnown(md protoreflect.MessageDescriptor) bool {
	return strings.HasPrefix(string(md.FullName()), "google.protobuf.")
}

// resolveMessage follows a path of field names, indexes and map keys from a
// message, and returns the message the path ends in, or nil.
func resolveMessage(md protoreflect.MessageDescriptor, path []string) protoreflect.MessageDescriptor {
	for i := 0; i < len(path); i++ {
		if md == nil || wellKnown(md) {
			return nil
		}
		f := findField(md, path[i])
		if f == nil {
			return nil
		}
		switch {
		case f.IsMap():
			if i+1 >= len(path) {
				return nil // a map key is being typed
			}
			i++ // the map key
			md = f.MapValue().Message()
		case f.IsList():
			if i+1 < len(path) && isNumeric(path[i+1]) {
				i++
			}
			md = f.Message()
		default:
			md = f.Message()
		}
	}
	if md != nil && wellKnown(md) {
		return nil
	}
	return md
}

// fieldType describes the type of a field.
func fieldType(f protoreflect.FieldDescriptor) string {
	kind := func(f protoreflect.FieldDescriptor) string {
		switch {
		case f.Message() != nil:
			return string(f.Message().FullName())
		case f.Enum() != nil:
			return string(f.Enum().FullName())
		default:
			return f.Kind().String()
		}
	}
	switch {
	case f.IsMap():
		return fmt.Sprintf("map<%s, %s>", kind(f.MapKey()), kind(f.MapValue()))
	case f.IsList():
		return "repeated " + kind(f)
	default:
		return kind(f)
	}
}

// comments returns the leading comments of a descriptor, when the source
// information was kept.
func comments(d protoreflect.Descriptor) string {
	return strings.TrimSpace(d.ParentFile().SourceLocations().ByDescriptor(d).LeadingComments)
}

// completeProto completes service and method names and message fields of a
// gRPC step that uses proto files. It reports false when it does not apply,
// so that the schema completion runs.
func (s *Server) completeProto(doc *document, ctx *yamlutil.CursorContext, pos Position) ([]CompletionItem, bool) {
	if doc.Parsed == nil || len(ctx.Path) < 3 || ctx.Path[0] != nsSteps {
		return nil, false
	}
	path := ctx.Path[1:]
	v := newYAMLView(doc.Parsed.File)
	step := stepAtLine(doc, v, pos.Line)
	if step == nil {
		return nil, false
	}
	fds := s.stepProto(doc, v, step)
	if fds == nil {
		return nil, false
	}
	var items []CompletionItem
	switch {
	case ctx.Type == yamlutil.CursorContextValue && slices.Equal(path, []string{"request", "service"}):
		names, _ := fds.ListServices()
		for _, n := range names {
			item := CompletionItem{Label: string(n), Kind: CompletionItemKindModule, Detail: "gRPC service"}
			if sd, err := fds.ResolveService(n); err == nil {
				item.Documentation = comments(sd)
			}
			items = append(items, item)
		}
	case ctx.Type == yamlutil.CursorContextValue && slices.Equal(path, []string{"request", "method"}):
		request := v.entries(v.entryValue(v.entries(step), "request"))
		sd, err := fds.ResolveService(protoreflect.FullName(v.stringValue(v.entryValue(request, "service"))))
		if err != nil {
			return nil, false
		}
		methods := sd.Methods()
		for i := range methods.Len() {
			m := methods.Get(i)
			items = append(items, CompletionItem{
				Label:         string(m.Name()),
				Kind:          CompletionItemKindFunction,
				Detail:        fmt.Sprintf("(%s) returns (%s)", m.Input().FullName(), m.Output().FullName()),
				Documentation: comments(m),
			})
		}
	case ctx.Type == yamlutil.CursorContextKey:
		msg, rest := messageOf(protoMethod(fds, v, step), path)
		md := resolveMessage(msg, rest)
		if md == nil {
			return nil, false
		}
		fields := md.Fields()
		for i := range fields.Len() {
			f := fields.Get(i)
			items = append(items, CompletionItem{
				Label:         f.JSONName(),
				Kind:          CompletionItemKindField,
				Detail:        fieldType(f),
				Documentation: comments(f),
			})
		}
	default:
		return nil, false
	}
	return items, len(items) > 0
}

// hoverProto describes the service, the method or the message field under
// the cursor.
func (s *Server) hoverProto(doc *document, nodePath *yamlutil.NodePath, pos Position) *Hover {
	keys := nodePath.Keys
	if doc.Parsed == nil || len(keys) < 4 || keys[0] != nsSteps || !isNumeric(keys[1]) {
		return nil
	}
	v := newYAMLView(doc.Parsed.File)
	starts := docStartLines(doc.Parsed.File)
	steps := v.sequence(v.entryValue(v.entries(doc.Parsed.File.Docs[docIndexAt(starts, pos.Line)].Body), nsSteps))
	idx := 0
	if _, err := fmt.Sscanf(keys[1], "%d", &idx); err != nil || idx >= len(steps) {
		return nil
	}
	fds := s.stepProto(doc, v, steps[idx])
	if fds == nil {
		return nil
	}
	path := keys[2:]
	var content string
	switch {
	case slices.Equal(path, []string{"request", "service"}):
		sd, err := fds.ResolveService(protoreflect.FullName(v.stringValue(v.entryValue(v.entries(v.entryValue(v.entries(steps[idx]), "request")), "service"))))
		if err != nil {
			return nil
		}
		content = fmt.Sprintf("**service** `%s`\n\n%s", sd.FullName(), comments(sd))
	case slices.Equal(path, []string{"request", "method"}):
		md := protoMethod(fds, v, steps[idx])
		if md == nil {
			return nil
		}
		content = fmt.Sprintf("**rpc** `%s(%s) returns (%s)`\n\n%s", md.Name(), md.Input().FullName(), md.Output().FullName(), comments(md))
	default:
		msg, rest := messageOf(protoMethod(fds, v, steps[idx]), path)
		if msg == nil || len(rest) == 0 {
			return nil
		}
		md := resolveMessage(msg, rest[:len(rest)-1])
		if md == nil {
			return nil
		}
		f := findField(md, rest[len(rest)-1])
		if f == nil {
			return nil
		}
		content = fmt.Sprintf("**%s** (JSON `%s`) `%s`\n\nField of `%s`\n\n%s", f.Name(), f.JSONName(), fieldType(f), md.FullName(), comments(f))
	}
	return &Hover{Contents: MarkupContent{Kind: "markdown", Value: strings.TrimSpace(content)}}
}

// protoDiagnostics reports unknown services, methods and message fields of
// the gRPC steps that use proto files.
func (s *Server) protoDiagnostics(doc *document) []Diagnostic {
	if doc.Parsed == nil || doc.Parsed.File == nil || doc.Parsed.Text != doc.Text {
		return nil
	}
	v := newYAMLView(doc.Parsed.File)
	c := &protoChecker{text: doc.Text, lines: newLineIndex(doc.Text), view: v}
	for _, d := range doc.Parsed.File.Docs {
		for _, step := range v.sequence(v.entryValue(v.entries(d.Body), nsSteps)) {
			if fds := s.stepProto(doc, v, step); fds != nil {
				c.checkStep(fds, step)
			}
		}
	}
	return c.diags
}

type protoChecker struct {
	text  string
	lines lineIndex
	view  *yamlView
	diags []Diagnostic
}

func (c *protoChecker) checkStep(fds grpcproto.FileDescriptors, step ast.Node) {
	v := c.view
	entries := v.entries(step)
	request := v.entries(v.entryValue(entries, "request"))
	service, ok := v.unwrap(v.entryValue(request, "service")).(*ast.StringNode)
	if !ok || strings.Contains(service.Value, "{{") {
		return
	}
	sd, err := fds.ResolveService(protoreflect.FullName(service.Value))
	if err != nil {
		c.warn(service, fmt.Sprintf("unknown gRPC service %q in the proto files", service.Value))
		return
	}
	method, ok := v.unwrap(v.entryValue(request, "method")).(*ast.StringNode)
	if !ok || strings.Contains(method.Value, "{{") {
		return
	}
	md := sd.Methods().ByName(protoreflect.Name(method.Value))
	if md == nil {
		c.warn(method, fmt.Sprintf("unknown method %q of %s", method.Value, sd.FullName()))
		return
	}
	expect := v.entries(v.entryValue(entries, "expect"))
	c.checkMessage(md.Input(), v.entryValue(request, "message"))
	c.checkMessage(md.Output(), v.entryValue(expect, "message"))
	for _, item := range v.sequence(v.entryValue(request, "messages")) {
		c.checkMessage(md.Input(), item)
	}
	for _, item := range v.sequence(v.entryValue(expect, "messages")) {
		c.checkMessage(md.Output(), item)
	}
}

// checkMessage checks the keys of a mapping that holds a message. A mapping
// with a template key (an assertion such as {{assert.and <-}}) and a
// template value are not checked.
func (c *protoChecker) checkMessage(md protoreflect.MessageDescriptor, node ast.Node) {
	if md == nil || wellKnown(md) {
		return
	}
	entries := c.view.entries(node)
	for _, mv := range entries {
		if mv.Key != nil && strings.Contains(mv.Key.String(), "{{") {
			return
		}
	}
	for _, mv := range entries {
		key, ok := mv.Key.(*ast.StringNode)
		if !ok {
			continue
		}
		f := findField(md, key.Value)
		if f == nil {
			c.warn(key, fmt.Sprintf("unknown field %q in %s", key.Value, md.FullName()))
			continue
		}
		c.checkFieldValue(f, mv.Value)
	}
}

func (c *protoChecker) checkFieldValue(f protoreflect.FieldDescriptor, node ast.Node) {
	switch {
	case f.IsMap():
		if vmd := f.MapValue().Message(); vmd != nil {
			for _, mv := range c.view.entries(node) {
				c.checkMessage(vmd, mv.Value)
			}
		}
	case f.Message() == nil:
	case f.IsList():
		for _, item := range c.view.sequence(node) {
			c.checkMessage(f.Message(), item)
		}
	default:
		c.checkMessage(f.Message(), node)
	}
}

func (c *protoChecker) warn(node *ast.StringNode, msg string) {
	r, ok := scalarRange(c.text, c.lines, node)
	if !ok {
		return
	}
	c.diags = append(c.diags, Diagnostic{Range: r, Severity: DiagnosticSeverityWarning, Message: msg})
}

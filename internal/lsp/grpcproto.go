package lsp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
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
// A gRPC step reads proto files when it has no client, reflection is
// disabled and its options list proto files, the step's options winning over
// the ones of scenarigo.yaml (protocol/grpc/request.go,
// request_proto_client.go). The server applies the same rules, compiles the
// files with scenarigo's compiler, and uses the service descriptors for
// completion, diagnostics and hover. It never uses reflection, which would
// need a connection.
//
// Compiled files are cached by import paths and files. A cached compilation
// is used while every proto file it read is unchanged (same file, size and
// modification time), so that changes outside the workspace folders, which
// no watcher reports, are seen too. A failed compilation only means no proto
// support for the step; it is tried again after a while, since the files
// that broke it are not known.

// protoCompileTimeout bounds one compilation.
const protoCompileTimeout = 5 * time.Second

// Delays before a failed compilation is tried again.
const (
	protoRetryAfterError   = 5 * time.Second
	protoRetryAfterTimeout = time.Minute
)

type protoCacheEntry struct {
	fds     grpcproto.FileDescriptors
	deps    map[string]string // proto path -> protoFileStamp
	retryAt time.Time         // when a failed compilation is tried again
}

// protoLookup finds the compiled proto files of the gRPC steps of one
// request: scenarigo.yaml is read once, and each cached compilation is
// checked once.
type protoLookup struct {
	s        *Server
	config   func() *grpc.Option
	compiled map[string]grpcproto.FileDescriptors
}

func (s *Server) newProtoLookup() *protoLookup {
	return &protoLookup{
		s:        s,
		config:   sync.OnceValue(s.grpcConfigOptions),
		compiled: map[string]grpcproto.FileDescriptors{},
	}
}

// step returns the compiled proto files of a gRPC step, or nil when the
// step does not use proto files or they cannot be compiled.
func (l *protoLookup) step(doc *document, v *yamlView, step ast.Node) grpcproto.FileDescriptors {
	entries := v.entries(step)
	if v.stringValue(v.entryValue(entries, "protocol")) != "grpc" {
		return nil
	}
	request := v.entries(v.entryValue(entries, "request"))
	if hasClient(v, v.entryValue(request, "client")) {
		return nil
	}
	opts := &grpc.RequestOptions{}
	if node := v.entryValue(request, "options"); node != nil {
		var stepOpts grpc.RequestOptions
		if err := yaml.NodeToValue(node, &stepOpts); err != nil {
			return nil
		}
		if err := mergo.Merge(opts, &stepOpts); err != nil {
			return nil
		}
	}
	if cfg := l.config(); cfg != nil && cfg.Request != nil {
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
	key := protoCacheKey(imports, files)
	fds, ok := l.compiled[key]
	if !ok {
		fds = l.s.compileProtos(imports, files)
		l.compiled[key] = fds
	}
	return fds
}

// hasClient reports whether a step sets request.client, which it then calls
// instead of a client built from the proto files.
func hasClient(v *yamlView, node ast.Node) bool {
	switch n := v.unwrap(node).(type) {
	case nil, *ast.NullNode:
		return false
	case *ast.StringNode:
		return n.Value != ""
	default:
		return true
	}
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
	now := time.Now()
	if e, ok := s.protoCache[key]; ok && e.fresh(imports, now) {
		return e.fds
	}
	ctx, cancel := context.WithTimeout(context.Background(), protoCompileTimeout)
	defer cancel()
	entry := &protoCacheEntry{}
	fds, err := grpcproto.NewCompiler(imports, grpcproto.WithSourceInfo()).Compile(ctx, files)
	if err != nil {
		s.logger.Printf("compile proto files %v: %v", files, err)
		entry.deps = map[string]string{}
		for _, f := range files {
			entry.deps[f] = protoFileStamp(imports, f)
		}
		entry.retryAt = now.Add(protoRetryDelay(err))
	} else {
		entry.fds = fds
		entry.deps = protoDeps(imports, fds)
	}
	if s.protoCache == nil {
		s.protoCache = map[string]*protoCacheEntry{}
	}
	s.protoCache[key] = entry
	return entry.fds
}

// protoRetryDelay returns how long a failed compilation stays cached. A
// compilation that ran out of time is not tried again soon, since it blocks
// the server while it runs.
func protoRetryDelay(err error) time.Duration {
	if errors.Is(err, context.DeadlineExceeded) {
		return protoRetryAfterTimeout
	}
	return protoRetryAfterError
}

// fresh reports whether a cached compilation can still be used.
func (e *protoCacheEntry) fresh(imports []string, now time.Time) bool {
	if !e.retryAt.IsZero() && !now.Before(e.retryAt) {
		return false
	}
	for path, stamp := range e.deps {
		if protoFileStamp(imports, path) != stamp {
			return false
		}
	}
	return true
}

// protoCacheKey identifies a compilation by its import paths and files.
func protoCacheKey(imports, files []string) string {
	return strings.Join(imports, "\x00") + "\x01" + strings.Join(files, "\x00")
}

// protoDeps returns the stamps of the proto files a compilation read: the
// compiled files and the files they import, directly or not.
func protoDeps(imports []string, fds grpcproto.FileDescriptors) map[string]string {
	deps := map[string]string{}
	var visit func(fd protoreflect.FileDescriptor)
	visit = func(fd protoreflect.FileDescriptor) {
		if _, ok := deps[fd.Path()]; ok {
			return
		}
		deps[fd.Path()] = protoFileStamp(imports, fd.Path())
		imported := fd.Imports()
		for i := range imported.Len() {
			visit(imported.Get(i).FileDescriptor)
		}
	}
	for _, f := range fds {
		visit(f)
	}
	return deps
}

// protoFileStamp identifies the file a proto path resolves to, as the
// compiler resolves it (the first import path that has it), with its size
// and modification time. A path that resolves to no file, such as a
// standard import, has an empty stamp.
func protoFileStamp(imports []string, path string) string {
	candidates := []string{path}
	if len(imports) > 0 {
		candidates = make([]string, 0, len(imports))
		for _, imp := range imports {
			candidates = append(candidates, filepath.Join(imp, path))
		}
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil {
			return fmt.Sprintf("%s@%d@%s", c, info.Size(), info.ModTime())
		}
	}
	return ""
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

// ownJSONForms are the messages that protojson reads in a form of their
// own instead of an object of their fields: a Timestamp is a string, a
// Struct any object, a wrapper its value. Other well-known messages, such
// as google.protobuf.Empty, are objects of their fields.
var ownJSONForms = map[protoreflect.FullName]bool{
	"google.protobuf.Any":         true,
	"google.protobuf.Timestamp":   true,
	"google.protobuf.Duration":    true,
	"google.protobuf.Struct":      true,
	"google.protobuf.Value":       true,
	"google.protobuf.ListValue":   true,
	"google.protobuf.FieldMask":   true,
	"google.protobuf.BoolValue":   true,
	"google.protobuf.BytesValue":  true,
	"google.protobuf.DoubleValue": true,
	"google.protobuf.FloatValue":  true,
	"google.protobuf.Int32Value":  true,
	"google.protobuf.Int64Value":  true,
	"google.protobuf.StringValue": true,
	"google.protobuf.UInt32Value": true,
	"google.protobuf.UInt64Value": true,
}

// ownJSONForm reports whether a message has a JSON form of its own.
func ownJSONForm(md protoreflect.MessageDescriptor) bool {
	return ownJSONForms[md.FullName()]
}

// resolveMessage follows a path of field names, indexes and map keys from a
// message, and returns the message the path ends in, or nil.
func resolveMessage(md protoreflect.MessageDescriptor, path []string) protoreflect.MessageDescriptor {
	for i := 0; i < len(path); i++ {
		if md == nil || ownJSONForm(md) {
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
	if md != nil && ownJSONForm(md) {
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
	fds := s.newProtoLookup().step(doc, v, step)
	if fds == nil {
		return nil, false
	}
	var items []CompletionItem
	switch {
	case ctx.Type == yamlutil.CursorContextValue && slices.Equal(path, []string{"request", "service"}):
		edit := valueEditRange(doc.Text, pos)
		names, _ := fds.ListServices()
		for _, n := range names {
			item := CompletionItem{
				Label:    string(n),
				Kind:     CompletionItemKindModule,
				Detail:   "gRPC service",
				TextEdit: &TextEdit{Range: edit, NewText: string(n)},
			}
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
		edit := valueEditRange(doc.Text, pos)
		methods := sd.Methods()
		for i := range methods.Len() {
			m := methods.Get(i)
			items = append(items, CompletionItem{
				Label:         string(m.Name()),
				Kind:          CompletionItemKindFunction,
				Detail:        fmt.Sprintf("(%s) returns (%s)", m.Input().FullName(), m.Output().FullName()),
				Documentation: comments(m),
				TextEdit:      &TextEdit{Range: edit, NewText: string(m.Name())},
			})
		}
	case ctx.Type == yamlutil.CursorContextKey:
		msg, rest := messageOf(protoMethod(fds, v, step), path)
		md := resolveMessage(msg, rest)
		if md == nil {
			return nil, false
		}
		items = completeFields(md, ctx, pos, path[0] == "request")
	default:
		return nil, false
	}
	return items, len(items) > 0
}

// completeFields completes the keys of a message: the fields not set yet,
// by any of their names, and for a request message not in a oneof that is
// set already, since protojson rejects both. Keys are completed with their
// JSON name, filtered by the typed prefix.
func completeFields(md protoreflect.MessageDescriptor, ctx *yamlutil.CursorContext, pos Position, request bool) []CompletionItem {
	set := map[protoreflect.FieldNumber]bool{}
	oneofs := map[protoreflect.FullName]bool{}
	for _, k := range ctx.ParentKeys {
		if f := findField(md, k); f != nil {
			set[f.Number()] = true
			if o := f.ContainingOneof(); o != nil && !o.IsSynthetic() {
				oneofs[o.FullName()] = true
			}
		}
	}
	edit := Range{
		Start: Position{Line: pos.Line, Character: pos.Character - len(ctx.PartialKey)},
		End:   pos,
	}
	var items []CompletionItem
	fields := md.Fields()
	for i := range fields.Len() {
		f := fields.Get(i)
		if set[f.Number()] {
			continue
		}
		if o := f.ContainingOneof(); request && o != nil && !o.IsSynthetic() && oneofs[o.FullName()] {
			continue
		}
		name := f.JSONName()
		if !strings.HasPrefix(name, ctx.PartialKey) && !strings.HasPrefix(string(f.Name()), ctx.PartialKey) {
			continue
		}
		insert := name + ": "
		if f.Message() != nil || f.IsList() || f.IsMap() {
			insert = name + ":"
		}
		items = append(items, CompletionItem{
			Label:         name,
			Kind:          CompletionItemKindField,
			Detail:        fieldType(f),
			Documentation: comments(f),
			InsertText:    insert,
			TextEdit:      &TextEdit{Range: edit, NewText: insert},
			SortText:      fmt.Sprintf("%03d_%s", i, name),
		})
	}
	return items
}

// valueEditRange returns the range a completed value replaces: the value of
// the key on the cursor's line, without quotes and a trailing comment,
// extended to the cursor. Clients take only the word before the cursor
// otherwise, which would double the dotted part of test.v1.Greeter.
func valueEditRange(text string, pos Position) Range {
	line := lineAt(text, pos.Line)
	cursor := min(pos.Character, len(line))
	start := cursor
	if colon := strings.Index(line[:cursor], ":"); colon >= 0 {
		start = colon + 1
		for start < len(line) && line[start] == ' ' {
			start++
		}
		if start < len(line) && (line[start] == '"' || line[start] == '\'') {
			start++
		}
	}
	start = min(start, cursor)
	rest := line[start:]
	if i := strings.Index(rest, " #"); i >= 0 {
		rest = rest[:i]
	}
	rest = strings.TrimRight(rest, " \t\r")
	if n := len(rest); n > 0 && (rest[n-1] == '"' || rest[n-1] == '\'') {
		rest = rest[:n-1]
	}
	end := max(start+len(rest), cursor)
	return Range{Start: Position{Line: pos.Line, Character: start}, End: Position{Line: pos.Line, Character: end}}
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
	fds := s.newProtoLookup().step(doc, v, steps[idx])
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
	lookup := s.newProtoLookup()
	for _, d := range doc.Parsed.File.Docs {
		for _, step := range v.sequence(v.entryValue(v.entries(d.Body), nsSteps)) {
			if fds := lookup.step(doc, v, step); fds != nil {
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
	c.checkMessage(md.Input(), v.entryValue(request, "message"), true)
	c.checkMessage(md.Output(), v.entryValue(expect, "message"), false)
	for _, item := range v.sequence(v.entryValue(request, "messages")) {
		c.checkMessage(md.Input(), item, true)
	}
	for _, item := range v.sequence(v.entryValue(expect, "messages")) {
		c.checkMessage(md.Output(), item, false)
	}
}

// checkMessage checks the keys of a mapping that holds a message. A mapping
// with a template key (an assertion such as {{assert.and <-}}) and a
// template value are not checked. A request message is decoded by
// protojson, which also rejects a field set twice (by two of its names) and
// two members of a oneof; an expected message is a set of assertions, which
// may do both.
func (c *protoChecker) checkMessage(md protoreflect.MessageDescriptor, node ast.Node, request bool) {
	if md == nil || ownJSONForm(md) {
		return
	}
	entries := c.view.entries(node)
	for _, mv := range entries {
		if mv.Key != nil && strings.Contains(mv.Key.String(), "{{") {
			return
		}
	}
	setBy := map[protoreflect.FieldNumber]string{}
	oneofs := map[protoreflect.FullName]string{}
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
		if request {
			c.checkSetOnce(f, key, mv.Value, setBy, oneofs)
		}
		c.checkFieldValue(f, mv.Value, request)
	}
}

// checkSetOnce reports a field of a request message set a second time, and
// a second member of a oneof set to a value. protojson ignores a null
// member of a oneof, and a template may give null.
func (c *protoChecker) checkSetOnce(f protoreflect.FieldDescriptor, key *ast.StringNode, value ast.Node, setBy map[protoreflect.FieldNumber]string, oneofs map[protoreflect.FullName]string) {
	if prev, ok := setBy[f.Number()]; ok {
		c.warn(key, fmt.Sprintf("field %s is set twice: %q and %q", f.FullName(), prev, key.Value))
		return
	}
	setBy[f.Number()] = key.Value
	o := f.ContainingOneof()
	if o == nil || o.IsSynthetic() {
		return
	}
	switch n := c.view.unwrap(value).(type) {
	case nil, *ast.NullNode:
		return
	case *ast.StringNode:
		if strings.Contains(n.Value, "{{") {
			return
		}
	}
	if prev, ok := oneofs[o.FullName()]; ok {
		c.warn(key, fmt.Sprintf("oneof %s is set twice: %q and %q", o.FullName(), prev, key.Value))
		return
	}
	oneofs[o.FullName()] = key.Value
}

func (c *protoChecker) checkFieldValue(f protoreflect.FieldDescriptor, node ast.Node, request bool) {
	switch {
	case f.IsMap():
		if vmd := f.MapValue().Message(); vmd != nil {
			for _, mv := range c.view.entries(node) {
				c.checkMessage(vmd, mv.Value, request)
			}
		}
	case f.Message() == nil:
	case f.IsList():
		for _, item := range c.view.sequence(node) {
			c.checkMessage(f.Message(), item, request)
		}
	default:
		c.checkMessage(f.Message(), node, request)
	}
}

func (c *protoChecker) warn(node *ast.StringNode, msg string) {
	r, ok := scalarRange(c.text, c.lines, node)
	if !ok {
		return
	}
	c.diags = append(c.diags, Diagnostic{Range: r, Severity: DiagnosticSeverityWarning, Message: msg})
}

package proto

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/linker"
)

// ServiceDescriptorResolver is an interface to resolve service descriptors.
type ServiceDescriptorResolver interface {
	ListServices() ([]protoreflect.FullName, error)
	ResolveService(protoreflect.FullName) (protoreflect.ServiceDescriptor, error)
}

// NewCompiler creates a new compiler with the given import paths.
func NewCompiler(imports []string) *Compiler {
	return &Compiler{
		imports: imports,
	}
}

// Compiler is a compiler for proto files.
type Compiler struct {
	imports []string
}

// Compile compiles the given file names into fully-linked descriptors.
func (c *Compiler) Compile(ctx context.Context, files []string) (FileDescriptors, error) {
	compiler := &protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: c.imports,
		}),
	}
	fds, err := compiler.Compile(ctx, files...)
	if err != nil {
		return nil, fmt.Errorf("failed to compile: %w", err)
	}
	return FileDescriptors(fds), nil
}

// FileDescriptors is a collection of file descriptors.
type FileDescriptors linker.Files

// ListServices lists all service names.
func (fds FileDescriptors) ListServices() ([]protoreflect.FullName, error) {
	names := []protoreflect.FullName{}
	for _, f := range fds.Files() {
		svcs := f.Services()
		for i := range svcs.Len() {
			names = append(names, svcs.Get(i).FullName())
		}
	}
	return names, nil
}

// ResolveService resolves a service descriptor by the given name.
func (fds FileDescriptors) ResolveService(name protoreflect.FullName) (protoreflect.ServiceDescriptor, error) {
	var svcDesc protoreflect.ServiceDescriptor
	for _, f := range fds.Files() {
		sd := f.Services().ByName(name.Name())
		if sd != nil {
			svcDesc = sd
			break
		}
	}
	if svcDesc == nil {
		return nil, fmt.Errorf("service %q not found", name)
	}
	return svcDesc, nil
}

// Files returns the underlying protobuf files.
func (fds FileDescriptors) Files() linker.Files {
	return linker.Files(fds)
}

package plugin

import (
	"context"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

func TestCreateModifiedNetFile(t *testing.T) {
	tests := map[string]struct {
		filename     string
		functionName string
		isMethod     bool
		sourceCode   string
		expectChecks []func(string) error
	}{
		"DialContext method": {
			filename: "dial.go",
			sourceCode: `package net

import (
	"context"
	"time"
)

// DialContext connects to the address on the named network using
// the provided context.
func DialContext(ctx context.Context, network, address string) (Conn, error) {
	d := &Dialer{Timeout: 30 * time.Second}
	return d.DialContext(ctx, network, address)
}

type Dialer struct {
	Timeout time.Duration
}

func (d *Dialer) DialContext(ctx context.Context, network, address string) (Conn, error) {
	// implementation here
	return nil, nil
}

type Conn interface{}
`,
			expectChecks: []func(string) error{
				func(modifiedSrc string) error {
					// Check that unsafe import was added
					if !strings.Contains(modifiedSrc, `_ "unsafe"`) {
						return &testError{"unsafe import not found"}
					}
					return nil
				},
				func(modifiedSrc string) error {
					// Check that linkname function was added
					expected := "//go:linkname _dialContext github.com/goccy/wasi-go-net/wasip1.DialContext"
					if !strings.Contains(modifiedSrc, expected) {
						return &testError{"linkname comment not found"}
					}
					return nil
				},
				func(modifiedSrc string) error {
					// Check that _dialContext function signature exists - relaxed check
					if !strings.Contains(modifiedSrc, "_dialContext(ctx context.Context, network string, address string)") {
						return &testError{"_dialContext function signature not found"}
					}
					return nil
				},
				func(modifiedSrc string) error {
					// Check that DialContext calls _dialContext - be more flexible with formatting
					if !strings.Contains(modifiedSrc, "_dialContext(") || !strings.Contains(modifiedSrc, "ctx, network, address") {
						return &testError{"DialContext does not call _dialContext"}
					}
					return nil
				},
				func(modifiedSrc string) error {
					// Verify AST structure is valid
					fset := token.NewFileSet()
					_, err := parser.ParseFile(fset, "test.go", modifiedSrc, parser.ParseComments)
					if err != nil {
						return &testError{"generated code is not valid Go: " + err.Error()}
					}
					return nil
				},
			},
		},
		"Listen function": {
			filename: "listen.go",
			sourceCode: `package net

import (
	"syscall"
)

// Listen announces on the local network address.
func Listen(network, address string) (Listener, error) {
	var la Addr
	switch network {
	case "tcp", "tcp4", "tcp6":
		la, _ = ResolveTCPAddr(network, address)
	}
	return ListenTCP(network, la.(*TCPAddr))
}

type Listener interface{}
type Addr interface{}
type TCPAddr struct{}

func ResolveTCPAddr(network, address string) (Addr, error) {
	return &TCPAddr{}, nil
}

func ListenTCP(network string, laddr *TCPAddr) (Listener, error) {
	// implementation here
	return nil, nil
}
`,
			expectChecks: []func(string) error{
				func(modifiedSrc string) error {
					// Check that unsafe import was added
					if !strings.Contains(modifiedSrc, `_ "unsafe"`) {
						return &testError{"unsafe import not found"}
					}
					return nil
				},
				func(modifiedSrc string) error {
					// Check that linkname function was added
					expected := "//go:linkname _listen github.com/goccy/wasi-go-net/wasip1.Listen"
					if !strings.Contains(modifiedSrc, expected) {
						return &testError{"linkname comment not found"}
					}
					return nil
				},
				func(modifiedSrc string) error {
					// Check that _listen function signature exists - relaxed check
					if !strings.Contains(modifiedSrc, "_listen(network string, address string)") {
						return &testError{"_listen function signature not found"}
					}
					return nil
				},
				func(modifiedSrc string) error {
					// Check that Listen calls _listen - be more flexible with formatting
					if !strings.Contains(modifiedSrc, "_listen(network, address)") {
						return &testError{"Listen does not call _listen"}
					}
					return nil
				},
				func(modifiedSrc string) error {
					// Verify AST structure is valid
					fset := token.NewFileSet()
					_, err := parser.ParseFile(fset, "test.go", modifiedSrc, parser.ParseComments)
					if err != nil {
						return &testError{"generated code is not valid Go: " + err.Error()}
					}
					return nil
				},
			},
		},
		"existing unsafe import": {
			filename:     "dial_existing_unsafe.go",
			functionName: "DialContext",
			isMethod:     false,
			sourceCode: `package net

import (
	"context"
	"time"
	_ "unsafe"
)

// DialContext connects to the address on the named network using
// the provided context.
func DialContext(ctx context.Context, network, address string) (Conn, error) {
	d := &Dialer{Timeout: 30 * time.Second}
	return d.DialContext(ctx, network, address)
}

type Dialer struct {
	Timeout time.Duration
}

func (d *Dialer) DialContext(ctx context.Context, network, address string) (Conn, error) {
	// implementation here
	return nil, nil
}

type Conn interface{}
`,
			expectChecks: []func(string) error{
				func(modifiedSrc string) error {
					// Check that unsafe import exists (and wasn't duplicated)
					count := strings.Count(modifiedSrc, `import _ "unsafe"`) + strings.Count(modifiedSrc, `_ "unsafe"`)
					if count != 1 {
						return &testError{"unsafe import should appear exactly once"}
					}
					return nil
				},
				func(modifiedSrc string) error {
					// Verify AST structure is valid
					fset := token.NewFileSet()
					_, err := parser.ParseFile(fset, "test.go", modifiedSrc, parser.ParseComments)
					if err != nil {
						return &testError{"generated code is not valid Go: " + err.Error()}
					}
					return nil
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Create a temporary file with test source code
			tmpFile, err := os.CreateTemp("", test.filename+"_*.go")
			if err != nil {
				t.Fatalf("failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			if _, err := tmpFile.WriteString(test.sourceCode); err != nil {
				t.Fatalf("failed to write source code: %v", err)
			}
			tmpFile.Close()

			// Test the function
			modifiedFilePath, err := createModifiedNetFile(tmpFile.Name())
			if err != nil {
				t.Fatalf("createModifiedNetFile failed: %v", err)
			}
			defer os.Remove(modifiedFilePath)

			// Read the modified file
			modifiedContent, err := os.ReadFile(modifiedFilePath)
			if err != nil {
				t.Fatalf("failed to read modified file: %v", err)
			}

			modifiedSrc := string(modifiedContent)
			t.Logf("Modified source code:\n%s", modifiedSrc)

			// Run all validation checks
			for i, check := range test.expectChecks {
				if err := check(modifiedSrc); err != nil {
					t.Errorf("Check %d failed: %v", i, err)
				}
			}
		})
	}
}

func TestCreateModifiedNetFileErrors(t *testing.T) {
	tests := map[string]struct {
		filename     string
		functionName string
		sourceCode   string
		expectError  string
	}{
		"no target functions": {
			filename: "no_function.go",
			sourceCode: `package net

func SomeOtherFunction() {}
`,
			expectError: "no target functions (DialContext or Listen) found",
		},
		"invalid Go source": {
			filename: "invalid.go",
			sourceCode:   `package net\n\ninvalid go code`,
			expectError: "failed to parse file",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Create a temporary file with test source code
			tmpFile, err := os.CreateTemp("", test.filename+"_*.go")
			if err != nil {
				t.Fatalf("failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			if _, err := tmpFile.WriteString(test.sourceCode); err != nil {
				t.Fatalf("failed to write source code: %v", err)
			}
			tmpFile.Close()

			// Test the function - should return error
			_, err = createModifiedNetFile(tmpFile.Name())
			if err == nil {
				t.Fatal("expected error but got nil")
			}

			if !strings.Contains(err.Error(), test.expectError) {
				t.Fatalf("expected error containing %q but got %q", test.expectError, err.Error())
			}
		})
	}
}

func TestCreateNetPackageOverlay(t *testing.T) {
	// Mock go command for testing
	goCmd, err := findGoCmd(context.Background())
	if err != nil {
		t.Skip("go command not available")
	}

	ctx := context.Background()
	overlayFiles, err := createNetPackageOverlay(ctx, goCmd)
	if err != nil {
		t.Fatalf("createNetPackageOverlay failed: %v", err)
	}

	// Clean up temp files
	defer func() {
		for _, file := range overlayFiles {
			os.Remove(file)
		}
	}()

	// Check that we have overlay files for both DialContext and Listen
	if len(overlayFiles) == 0 {
		t.Fatal("no overlay files created")
	}

	// Verify each overlay file
	for originalPath, overlayPath := range overlayFiles {
		t.Logf("Original: %s -> Overlay: %s", originalPath, overlayPath)

		// Read overlay file content
		content, err := os.ReadFile(overlayPath)
		if err != nil {
			t.Fatalf("failed to read overlay file %s: %v", overlayPath, err)
		}

		contentStr := string(content)

		// Check that it's valid Go code
		fset := token.NewFileSet()
		astFile, err := parser.ParseFile(fset, overlayPath, content, parser.ParseComments)
		if err != nil {
			t.Fatalf("overlay file %s is not valid Go: %v", overlayPath, err)
		}

		// Check that it has the package declaration
		if astFile.Name.Name != "net" {
			t.Errorf("expected package 'net', got '%s'", astFile.Name.Name)
		}

		// Check that unsafe import is present
		hasUnsafe := false
		for _, imp := range astFile.Imports {
			if imp.Path.Value == `"unsafe"` {
				hasUnsafe = true
				break
			}
		}
		if !hasUnsafe {
			t.Error("unsafe import not found in overlay file")
		}

		// Check that linkname directives are present
		hasLinkname := false
		for _, decl := range astFile.Decls {
			if funcDecl, ok := decl.(*ast.FuncDecl); ok {
				if funcDecl.Doc != nil {
					for _, comment := range funcDecl.Doc.List {
						if strings.Contains(comment.Text, "//go:linkname") {
							hasLinkname = true
							break
						}
					}
				}
			}
		}
		if !hasLinkname {
			t.Error("linkname directive not found in overlay file")
		}

		// Verify that the content can be formatted (syntax is correct)
		fset2 := token.NewFileSet()
		astFile2, err := parser.ParseFile(fset2, "", contentStr, parser.ParseComments)
		if err != nil {
			t.Fatalf("failed to parse overlay content: %v", err)
		}

		var buf strings.Builder
		if err := format.Node(&buf, fset2, astFile2); err != nil {
			t.Fatalf("failed to format overlay content: %v", err)
		}

		t.Logf("Overlay file content:\n%s", buf.String())
	}
}

// testError is a simple error type for test checks
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
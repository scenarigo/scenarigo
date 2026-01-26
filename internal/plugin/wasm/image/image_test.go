package image

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
)

var testWASMBytes = []byte{
	0x00, 0x61, 0x73, 0x6d, // WASM magic number
	0x01, 0x00, 0x00, 0x00, // WASM version 1
}

func TestBuildFromBytes(t *testing.T) {
	tests := []struct {
		name    string
		wasm    []byte
		opts    []BuildOption
		wantCfg *Config
	}{
		{
			name: "default config",
			wasm: testWASMBytes,
			wantCfg: &Config{
				Type: "compat",
			},
		},
		{
			name: "custom config",
			wasm: testWASMBytes,
			opts: []BuildOption{
				WithConfig(&Config{
					Type:        "scenarigo",
					ABIVersions: []string{"v1"},
					Config:      map[string]any{"key": "value"},
				}),
			},
			wantCfg: &Config{
				Type:        "scenarigo",
				ABIVersions: []string{"v1"},
				Config:      map[string]any{"key": "value"},
			},
		},
		{
			name: "with annotations",
			wasm: testWASMBytes,
			opts: []BuildOption{
				WithAnnotations(map[string]string{
					"org.opencontainers.image.title": "test",
				}),
			},
			wantCfg: &Config{
				Type: "compat",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img, err := BuildFromBytes(tt.wasm, tt.opts...)
			if err != nil {
				t.Fatalf("BuildFromBytes() error = %v", err)
			}
			if img == nil {
				t.Fatal("BuildFromBytes() returned nil image")
			}

			cfg, err := ExtractConfig(img)
			if err != nil {
				t.Fatalf("ExtractConfig() error = %v", err)
			}
			if cfg.Type != tt.wantCfg.Type {
				t.Errorf("Config.Type = %v, want %v", cfg.Type, tt.wantCfg.Type)
			}
		})
	}
}

func TestExtractWASM(t *testing.T) {
	img, err := BuildFromBytes(testWASMBytes)
	if err != nil {
		t.Fatalf("BuildFromBytes() error = %v", err)
	}

	extracted, err := ExtractWASM(img)
	if err != nil {
		t.Fatalf("ExtractWASM() error = %v", err)
	}

	if !bytes.Equal(extracted, testWASMBytes) {
		t.Errorf("ExtractWASM() = %v, want %v", extracted, testWASMBytes)
	}
}

func TestExtractConfig(t *testing.T) {
	wantCfg := &Config{
		Type:        "scenarigo",
		ABIVersions: []string{"v1", "v2"},
		Config:      map[string]any{"foo": "bar"},
	}

	img, err := BuildFromBytes(testWASMBytes, WithConfig(wantCfg))
	if err != nil {
		t.Fatalf("BuildFromBytes() error = %v", err)
	}

	cfg, err := ExtractConfig(img)
	if err != nil {
		t.Fatalf("ExtractConfig() error = %v", err)
	}

	if cfg.Type != wantCfg.Type {
		t.Errorf("Config.Type = %v, want %v", cfg.Type, wantCfg.Type)
	}
	if len(cfg.ABIVersions) != len(wantCfg.ABIVersions) {
		t.Errorf("Config.ABIVersions = %v, want %v", cfg.ABIVersions, wantCfg.ABIVersions)
	}
	if cfg.Config["foo"] != wantCfg.Config["foo"] {
		t.Errorf("Config.Config[foo] = %v, want %v", cfg.Config["foo"], wantCfg.Config["foo"])
	}
}

func TestPushPull(t *testing.T) {
	reg := registry.New()
	server := httptest.NewServer(reg)
	defer server.Close()

	cfg := &Config{
		Type:        "scenarigo",
		ABIVersions: []string{"v1"},
		Config:      map[string]any{"test": "value"},
	}

	img, err := BuildFromBytes(testWASMBytes, WithConfig(cfg))
	if err != nil {
		t.Fatalf("BuildFromBytes() error = %v", err)
	}

	ref := strings.TrimPrefix(server.URL, "http://") + "/test/plugin:v1"

	ctx := context.Background()
	digestRef, err := Push(ctx, img, ref, WithInsecure(true))
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if digestRef == "" {
		t.Fatal("Push() returned empty digest reference")
	}

	pulled, err := Pull(ctx, ref, WithPullInsecure(true))
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}

	extractedWASM, err := ExtractWASM(pulled)
	if err != nil {
		t.Fatalf("ExtractWASM() error = %v", err)
	}
	if !bytes.Equal(extractedWASM, testWASMBytes) {
		t.Errorf("ExtractWASM() = %v, want %v", extractedWASM, testWASMBytes)
	}
}

func TestBuildError(t *testing.T) {
	t.Run("non-existent file", func(t *testing.T) {
		_, err := Build("/non/existent/file.wasm")
		if err == nil {
			t.Fatal("Build() expected error for non-existent file")
		}
	})
}

func TestPushError(t *testing.T) {
	t.Run("invalid reference", func(t *testing.T) {
		img, _ := BuildFromBytes(testWASMBytes)
		_, err := Push(context.Background(), img, "INVALID:ref::")
		if err == nil {
			t.Fatal("Push() expected error for invalid reference")
		}
	})
}

func TestPullError(t *testing.T) {
	t.Run("invalid reference", func(t *testing.T) {
		_, err := Pull(context.Background(), "INVALID:ref::")
		if err == nil {
			t.Fatal("Pull() expected error for invalid reference")
		}
	})

	t.Run("non-existent image", func(t *testing.T) {
		reg := registry.New()
		server := httptest.NewServer(reg)
		defer server.Close()

		ref := strings.TrimPrefix(server.URL, "http://") + "/does/not/exist:latest"
		_, err := Pull(context.Background(), ref, WithPullInsecure(true))
		if err == nil {
			t.Fatal("Pull() expected error for non-existent image")
		}
	})
}

func TestMediaTypes(t *testing.T) {
	if ConfigMediaType != "application/vnd.module.wasm.config.v1+json" {
		t.Errorf("ConfigMediaType = %v, want %v", ConfigMediaType, "application/vnd.module.wasm.config.v1+json")
	}
	if ContentMediaType != "application/vnd.module.wasm.content.layer.v1+wasm" {
		t.Errorf("ContentMediaType = %v, want %v", ContentMediaType, "application/vnd.module.wasm.content.layer.v1+wasm")
	}
}

func TestPushPullWithDigest(t *testing.T) {
	reg := registry.New()
	server := httptest.NewServer(reg)
	defer server.Close()

	img, err := BuildFromBytes(testWASMBytes)
	if err != nil {
		t.Fatalf("BuildFromBytes() error = %v", err)
	}

	ref := strings.TrimPrefix(server.URL, "http://") + "/test/plugin:v1"
	ctx := context.Background()

	digestRef, err := Push(ctx, img, ref, WithInsecure(true))
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	if !strings.Contains(digestRef, "@sha256:") {
		t.Errorf("Push() digest reference = %v, expected to contain @sha256:", digestRef)
	}

	pulledByDigest, err := Pull(ctx, digestRef, WithPullInsecure(true))
	if err != nil {
		t.Fatalf("Pull() by digest error = %v", err)
	}

	extractedWASM, err := ExtractWASM(pulledByDigest)
	if err != nil {
		t.Fatalf("ExtractWASM() error = %v", err)
	}
	if !bytes.Equal(extractedWASM, testWASMBytes) {
		t.Errorf("ExtractWASM() = %v, want %v", extractedWASM, testWASMBytes)
	}
}

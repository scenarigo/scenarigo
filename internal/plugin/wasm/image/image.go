package image

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// Type aliases for 3rd party types
type (
	// Image represents an OCI image.
	Image = v1.Image

	// MediaType represents an OCI media type.
	MediaType string
)

// Media types from solo-io/wasm-image-spec.
const (
	ConfigMediaType  MediaType = "application/vnd.module.wasm.config.v1+json"
	ContentMediaType MediaType = "application/vnd.module.wasm.content.layer.v1+wasm"
)

// Config is the WASM module configuration.
type Config struct {
	Type        string         `json:"type"`
	ABIVersions []string       `json:"abiVersions,omitempty"`
	Config      map[string]any `json:"config,omitempty"`
}

// buildOptions holds options for Build operations.
type buildOptions struct {
	config      *Config
	annotations map[string]string
}

// BuildOption configures Build operations.
type BuildOption func(*buildOptions)

// WithConfig sets the WASM module configuration.
func WithConfig(cfg *Config) BuildOption {
	return func(o *buildOptions) {
		o.config = cfg
	}
}

// WithAnnotations sets annotations on the image manifest.
func WithAnnotations(annotations map[string]string) BuildOption {
	return func(o *buildOptions) {
		o.annotations = annotations
	}
}

// pushOptions holds options for Push operations.
type pushOptions struct {
	transport http.RoundTripper
	auth      authn.Authenticator
	keychain  authn.Keychain
	insecure  bool
}

func (o *pushOptions) toRemoteOptions() []remote.Option {
	var opts []remote.Option
	if o.transport != nil {
		opts = append(opts, remote.WithTransport(o.transport))
	}
	if o.auth != nil {
		opts = append(opts, remote.WithAuth(o.auth))
	}
	if o.keychain != nil {
		opts = append(opts, remote.WithAuthFromKeychain(o.keychain))
	}
	return opts
}

// PushOption configures Push operations.
type PushOption func(*pushOptions)

// WithTransport sets the HTTP transport for registry communication.
func WithTransport(transport http.RoundTripper) PushOption {
	return func(o *pushOptions) {
		o.transport = transport
	}
}

// WithAuth sets basic authentication credentials.
func WithAuth(username, password string) PushOption {
	return func(o *pushOptions) {
		o.auth = &authn.Basic{
			Username: username,
			Password: password,
		}
	}
}

// WithInsecure allows communication with insecure (HTTP) registries.
func WithInsecure(insecure bool) PushOption {
	return func(o *pushOptions) {
		o.insecure = insecure
	}
}

// WithKeychain sets the keychain for authentication.
func WithKeychain(keychain authn.Keychain) PushOption {
	return func(o *pushOptions) {
		o.keychain = keychain
	}
}

// pullOptions holds options for Pull operations.
type pullOptions struct {
	transport http.RoundTripper
	auth      authn.Authenticator
	keychain  authn.Keychain
	insecure  bool
}

func (o *pullOptions) toRemoteOptions() []remote.Option {
	var opts []remote.Option
	if o.transport != nil {
		opts = append(opts, remote.WithTransport(o.transport))
	}
	if o.auth != nil {
		opts = append(opts, remote.WithAuth(o.auth))
	}
	if o.keychain != nil {
		opts = append(opts, remote.WithAuthFromKeychain(o.keychain))
	}
	return opts
}

// PullOption configures Pull operations.
type PullOption func(*pullOptions)

// WithPullTransport sets the HTTP transport for registry communication.
func WithPullTransport(transport http.RoundTripper) PullOption {
	return func(o *pullOptions) {
		o.transport = transport
	}
}

// WithPullAuth sets basic authentication credentials.
func WithPullAuth(username, password string) PullOption {
	return func(o *pullOptions) {
		o.auth = &authn.Basic{
			Username: username,
			Password: password,
		}
	}
}

// WithPullInsecure allows communication with insecure (HTTP) registries.
func WithPullInsecure(insecure bool) PullOption {
	return func(o *pullOptions) {
		o.insecure = insecure
	}
}

// WithPullKeychain sets the keychain for authentication.
func WithPullKeychain(keychain authn.Keychain) PullOption {
	return func(o *pullOptions) {
		o.keychain = keychain
	}
}

// Build creates an OCI image from a WASM file.
func Build(wasmPath string, opts ...BuildOption) (Image, error) {
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read wasm file: %w", err)
	}
	return BuildFromBytes(wasmBytes, opts...)
}

// BuildFromBytes creates an OCI image from WASM bytes.
func BuildFromBytes(wasmBytes []byte, opts ...BuildOption) (Image, error) {
	options := &buildOptions{
		config: &Config{Type: "compat"},
	}
	for _, opt := range opts {
		opt(options)
	}

	configBytes, err := json.Marshal(options.config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	wasmLayer := &staticLayer{
		content:   wasmBytes,
		mediaType: types.MediaType(ContentMediaType),
	}

	img := empty.Image
	img, err = mutate.AppendLayers(img, wasmLayer)
	if err != nil {
		return nil, fmt.Errorf("failed to append wasm layer: %w", err)
	}

	img = mutate.ConfigMediaType(img, types.MediaType(ConfigMediaType))

	img, err = mutate.Append(img, mutate.Addendum{
		Layer: &staticLayer{
			content:   configBytes,
			mediaType: types.MediaType(ConfigMediaType),
		},
		History: v1.History{},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to append config layer: %w", err)
	}

	configFile, err := img.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("failed to get config file: %w", err)
	}
	configFile.Config.Labels = map[string]string{}
	for k, v := range options.config.Config {
		if s, ok := v.(string); ok {
			configFile.Config.Labels[k] = s
		}
	}
	img, err = mutate.ConfigFile(img, configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to set config file: %w", err)
	}

	if len(options.annotations) > 0 {
		img = mutate.Annotations(img, options.annotations).(Image)
	}

	return &wasmImage{
		Image:       img,
		configBytes: configBytes,
	}, nil
}

// Push pushes an OCI image to a registry.
func Push(ctx context.Context, img Image, ref string, opts ...PushOption) (string, error) {
	options := &pushOptions{}
	for _, opt := range opts {
		opt(options)
	}

	nameOpts := []name.Option{}
	if options.insecure {
		nameOpts = append(nameOpts, name.Insecure)
	}

	reference, err := name.ParseReference(ref, nameOpts...)
	if err != nil {
		return "", fmt.Errorf("failed to parse reference: %w", err)
	}

	remoteOpts := options.toRemoteOptions()
	remoteOpts = append(remoteOpts, remote.WithContext(ctx))

	if err := remote.Write(reference, img, remoteOpts...); err != nil {
		return "", fmt.Errorf("failed to push image: %w", err)
	}

	digest, err := img.Digest()
	if err != nil {
		return "", fmt.Errorf("failed to get image digest: %w", err)
	}

	return reference.Context().Digest(digest.String()).String(), nil
}

// Pull pulls an OCI image from a registry.
func Pull(ctx context.Context, ref string, opts ...PullOption) (Image, error) {
	options := &pullOptions{}
	for _, opt := range opts {
		opt(options)
	}

	nameOpts := []name.Option{}
	if options.insecure {
		nameOpts = append(nameOpts, name.Insecure)
	}

	reference, err := name.ParseReference(ref, nameOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to parse reference: %w", err)
	}

	remoteOpts := options.toRemoteOptions()
	remoteOpts = append(remoteOpts, remote.WithContext(ctx))

	img, err := remote.Image(reference, remoteOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to pull image: %w", err)
	}

	return img, nil
}

// ExtractWASM extracts the WASM bytes from an OCI image.
func ExtractWASM(img Image) ([]byte, error) {
	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("failed to get layers: %w", err)
	}

	for _, layer := range layers {
		mt, err := layer.MediaType()
		if err != nil {
			continue
		}
		if MediaType(mt) == ContentMediaType {
			rc, err := layer.Uncompressed()
			if err != nil {
				return nil, fmt.Errorf("failed to get layer content: %w", err)
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}

	return nil, fmt.Errorf("no wasm layer found in image")
}

// ExtractConfig extracts the Config from an OCI image.
func ExtractConfig(img Image) (*Config, error) {
	if wi, ok := img.(*wasmImage); ok {
		var cfg Config
		if err := json.Unmarshal(wi.configBytes, &cfg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config: %w", err)
		}
		return &cfg, nil
	}

	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("failed to get layers: %w", err)
	}

	for _, layer := range layers {
		mt, err := layer.MediaType()
		if err != nil {
			continue
		}
		if MediaType(mt) == ConfigMediaType {
			rc, err := layer.Uncompressed()
			if err != nil {
				return nil, fmt.Errorf("failed to get config layer content: %w", err)
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, fmt.Errorf("failed to read config layer: %w", err)
			}
			var cfg Config
			if err := json.Unmarshal(data, &cfg); err != nil {
				return nil, fmt.Errorf("failed to unmarshal config: %w", err)
			}
			return &cfg, nil
		}
	}

	return nil, fmt.Errorf("no config layer found in image")
}

// wasmImage wraps an image with config bytes for extraction.
type wasmImage struct {
	Image
	configBytes []byte
}

// staticLayer implements v1.Layer for static content.
type staticLayer struct {
	content   []byte
	mediaType types.MediaType
}

func (l *staticLayer) Digest() (v1.Hash, error) {
	h := sha256.Sum256(l.content)
	return v1.Hash{
		Algorithm: "sha256",
		Hex:       hex.EncodeToString(h[:]),
	}, nil
}

func (l *staticLayer) DiffID() (v1.Hash, error) {
	return l.Digest()
}

func (l *staticLayer) Compressed() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(l.content)), nil
}

func (l *staticLayer) Uncompressed() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(l.content)), nil
}

func (l *staticLayer) Size() (int64, error) {
	return int64(len(l.content)), nil
}

func (l *staticLayer) MediaType() (types.MediaType, error) {
	return l.mediaType, nil
}

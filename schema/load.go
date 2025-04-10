package schema

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	ytt "carvel.dev/ytt/pkg/cmd/template"
	yttui "carvel.dev/ytt/pkg/cmd/ui"
	yttfiles "carvel.dev/ytt/pkg/files"
	"github.com/fatih/color"
	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"

	"github.com/scenarigo/scenarigo/errors"
	"github.com/scenarigo/scenarigo/internal/filepathutil"
	"slices"
)

// LoadScenarios loads test scenarios from path.
func LoadScenarios(path string, opts ...LoadOption) ([]*Scenario, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return loadScenarios(path, b, opts...)
}

// LoadScenariosFromReader loads test scenarios with io.Reader.
func LoadScenariosFromReader(r io.Reader, opts ...LoadOption) ([]*Scenario, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read: %w", err)
	}
	return loadScenarios(filepath.Join(wd, "reader.yaml"), b, opts...)
}

func loadScenarios(path string, b []byte, opts ...LoadOption) ([]*Scenario, error) {
	var opt loadOption
	for _, o := range opts {
		if err := o(&opt); err != nil {
			return nil, err
		}
	}

	var dir string
	if opt.inputConfig.YAML.YTT.Enabled {
		f, err := yttfiles.NewFileFromSource(yttfiles.NewBytesSource(path, b))
		if err != nil {
			return nil, err
		}
		files := slices.Clone(opt.defaultYTTFiles)
		files = append(files, f)
		b, err = runYTT(opt.yttOpts, opt.yttUI, files...)
		if err != nil {
			return nil, fmt.Errorf("ytt failed: %w", err)
		}

		dir, err = filepath.Abs(filepath.Dir(path))
		if err != nil {
			return nil, fmt.Errorf("failed to get directory: %w", err)
		}
	}

	docs, err := readDocsWithSchemaVersionFromBytes(b)
	if err != nil {
		return nil, err
	}

	file := &ast.File{
		Name: path,
		Docs: []*ast.DocumentNode{},
	}
	for _, doc := range docs {
		switch doc.schemaVersion {
		case "ytt/v1":
			if !opt.inputConfig.YAML.YTT.Enabled {
				return nil, errors.WithNodeAndColored(
					errors.ErrorPath("schemaVersion", "ytt feature is not enabled"),
					doc.doc.Body,
					!color.NoColor,
				)
			}

			files := slices.Clone(opt.defaultYTTFiles)
			var y YTT
			if err := yaml.NodeToValue(doc.doc.Body, &y, yaml.Strict()); err != nil {
				return nil, err
			}
			fs, err := readYTTFiles(dir, y.Files...)
			if err != nil {
				return nil, fmt.Errorf("failed to read ytt files: %w", err)
			}
			files = append(files, fs...)

			b, err := runYTT(opt.yttOpts, opt.yttUI, files...)
			if err != nil {
				return nil, fmt.Errorf("ytt failed: %w", err)
			}

			f, err := parser.ParseBytes(b, 0)
			if err != nil {
				return nil, fmt.Errorf("failed to parse YAML: %w", err)
			}
			file.Docs = append(file.Docs, f.Docs...)
		case "", "scenario/v1":
			file.Docs = append(file.Docs, doc.doc)
		default:
			return nil, errors.WithNodeAndColored(
				errors.ErrorPathf("schemaVersion", "unknown version %q", doc.schemaVersion),
				doc.doc.Body,
				!color.NoColor,
			)
		}
	}

	return loadScenariosFromFileAST(file)
}

func runYTT(opts *ytt.Options, yttUI yttui.TTY, files ...*yttfiles.File) ([]byte, error) {
	input := ytt.Input{
		Files: files,
	}
	output := opts.RunWithFiles(input, yttUI)
	if output.Err != nil {
		return nil, output.Err
	}
	b, err := output.DocSet.AsBytes()
	if err != nil {
		return nil, err
	}
	return b, nil
}

func readYTTFiles(root string, paths ...string) ([]*yttfiles.File, error) {
	files := []*yttfiles.File{}
	for _, path := range paths {
		fs, err := findAllfiles(filepathutil.From(root, path))
		if err != nil {
			return nil, err
		}
		for _, f := range fs {
			b, err := os.ReadFile(f)
			if err != nil {
				return nil, err
			}
			src, err := yttfiles.NewFileFromSource(yttfiles.NewBytesSource(f, b))
			if err != nil {
				return nil, err
			}
			files = append(files, src)
		}
	}
	return files, nil
}

func findAllfiles(paths ...string) ([]string, error) {
	files := []string{}
	for _, path := range paths {
		if err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			files = append(files, path)
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return files, nil
}

func loadScenariosFromFileAST(f *ast.File) ([]*Scenario, error) {
	var buf bytes.Buffer
	dec := yaml.NewDecoder(&buf, yaml.UseOrderedMap(), yaml.Strict())
	var scenarios []*Scenario
	for _, doc := range f.Docs {
		var s Scenario
		if err := dec.DecodeFromNode(doc.Body, &s); err != nil {
			return nil, fmt.Errorf("failed to decode YAML: %w", err)
		}
		s.filepath = f.Name
		s.Node = doc.Body
		if err := s.Validate(); err != nil {
			return nil, fmt.Errorf("validation error: %s: %w", s.filepath, err)
		}
		scenarios = append(scenarios, &s)
	}
	return scenarios, nil
}

type loadOption struct {
	configRoot  string
	inputConfig InputConfig

	yttOpts         *ytt.Options
	yttUI           yttui.TTY
	defaultYTTFiles []*yttfiles.File
}

// LoadOption represents an option to load scenarios.
type LoadOption func(*loadOption) error

// WithInputConfig is an option to specify input config.
func WithInputConfig(root string, c InputConfig) func(*loadOption) error {
	return func(o *loadOption) error {
		o.configRoot = root
		o.inputConfig = c
		if c.YAML.YTT.Enabled {
			o.yttOpts = ytt.NewOptions()
			o.yttUI = yttui.NewCustomWriterTTY(false, io.Discard, io.Discard)
			defaultFiles, err := readYTTFiles(root, c.YAML.YTT.DefaultFiles...)
			if err != nil {
				return fmt.Errorf("failed to read default ytt files: %w", err)
			}
			o.defaultYTTFiles = defaultFiles
		}
		return nil
	}
}

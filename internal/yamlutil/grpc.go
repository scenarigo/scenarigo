package yamlutil

import (
	"encoding/hex"
	"strings"
	"unicode/utf8"

	"google.golang.org/grpc/metadata"

	"github.com/goccy/go-yaml"
)

func NewMDMarshaler(md metadata.MD) *MDMarshaler { return (*MDMarshaler)(&md) }

type MDMarshaler metadata.MD

func (m *MDMarshaler) MarshalYAML() ([]byte, error) {
	mp := make(metadata.MD, len(*m))
	for k, vs := range *m {
		if !strings.HasSuffix(k, "-bin") {
			mp[k] = vs
			continue
		}
		s := make([]string, len(vs))
		for i, v := range vs {
			if !utf8.ValidString(v) {
				v = hex.EncodeToString([]byte(v))
			}
			s[i] = v
		}
		mp[k] = s
	}
	return yaml.Marshal(mp)
}

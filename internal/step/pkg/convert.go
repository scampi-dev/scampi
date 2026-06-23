// SPDX-License-Identifier: GPL-3.0-only

package pkg

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"reflect"
	"strings"

	"scampi.dev/scampi/internal/lang/eval"
	"scampi.dev/scampi/internal/spec"
)

// Converters returns the type converters owned by the pkg step.
func Converters() spec.ConverterMap {
	return spec.ConverterMap{
		reflect.TypeFor[spec.PkgSourceRef](): ConvertPkgSourceRef,
	}
}

// ConvertPkgSourceRef converts a StructVal produced by pkg_system,
// pkg_apt_repo, or pkg_dnf_repo into a spec.PkgSourceRef.
func ConvertPkgSourceRef(typeName string, fields map[string]eval.Value, _ spec.ConvertContext) (any, error) {
	ref := spec.PkgSourceRef{}
	switch typeName {
	case "pkg_system":
		ref.Kind = spec.PkgSourceNative
	case "pkg_apt_repo":
		ref.Kind = spec.PkgSourceApt
		if u, ok := fields["url"].(*eval.StringVal); ok {
			ref.URL = u.V
			ref.Name = repoSlug(u.V)
		}
		if k, ok := fields["key_url"].(*eval.StringVal); ok {
			ref.KeyURL = k.V
		}
		if c, ok := fields["components"].(*eval.ListVal); ok {
			for _, item := range c.Items {
				if s, ok := item.(*eval.StringVal); ok {
					ref.Components = append(ref.Components, s.V)
				}
			}
		}
		if s, ok := fields["suite"].(*eval.StringVal); ok {
			ref.Suite = s.V
		}
	case "pkg_dnf_repo":
		ref.Kind = spec.PkgSourceDnf
		if u, ok := fields["url"].(*eval.StringVal); ok {
			ref.URL = u.V
			ref.Name = repoSlug(u.V)
		}
		if k, ok := fields["key_url"].(*eval.StringVal); ok {
			ref.KeyURL = k.V
		}
	}
	return ref, nil
}

func repoSlug(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		h := sha256.Sum256([]byte(rawURL))
		return hex.EncodeToString(h[:8])
	}
	host := strings.ReplaceAll(u.Hostname(), ".", "-")
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) > 0 && parts[0] != "" {
		return host + "-" + parts[0]
	}
	return host
}

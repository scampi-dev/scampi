// SPDX-License-Identifier: GPL-3.0-only

package star

import (
	"crypto/x509"
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"scampi.dev/scampi/spec"
	steprest "scampi.dev/scampi/step/rest"
	"scampi.dev/scampi/target/rest"
)

// restModule builds the `rest` namespace (rest.basic, rest.bearer, rest.header).
func restModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "rest",
		Members: starlark.StringDict{
			"request": starlark.NewBuiltin("rest.request", builtinRestRequest),
			"status":  starlark.NewBuiltin("rest.status", builtinRestStatus),
			"jq":      starlark.NewBuiltin("rest.jq", builtinRestJQ),
			"no_auth": starlark.NewBuiltin("rest.no_auth", builtinRestNoAuth),
			"basic":   starlark.NewBuiltin("rest.basic", builtinRestBasic),
			"bearer":  starlark.NewBuiltin("rest.bearer", builtinRestBearer),
			"header":  starlark.NewBuiltin("rest.header", builtinRestHeader),
			"body":    restBodyModule(),
			"tls":     restTLSModule(),
		},
	}
}

// starlarkAuth wraps an AuthConfig so it can be passed through Starlark as a value.
type starlarkAuth struct {
	config rest.AuthConfig
}

func (a starlarkAuth) String() string        { return "<rest.auth:" + a.config.Kind() + ">" }
func (a starlarkAuth) Type() string          { return "rest.auth" }
func (a starlarkAuth) Freeze()               {}
func (a starlarkAuth) Truth() starlark.Bool  { return starlark.True }
func (a starlarkAuth) Hash() (uint32, error) { return 0, nil }

// rest.no_auth()
func builtinRestNoAuth(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	if err := starlark.UnpackArgs("rest.no_auth", args, kwargs); err != nil {
		return nil, err
	}
	return starlarkAuth{config: rest.NoAuthConfig{}}, nil
}

// rest.basic(user, password)
func builtinRestBasic(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var user, password string
	if err := starlark.UnpackArgs("rest.basic", args, kwargs,
		"user", &user,
		"password", &password,
	); err != nil {
		return nil, err
	}
	return starlarkAuth{config: rest.BasicAuthConfig{User: user, Password: password}}, nil
}

// rest.bearer(token_endpoint, identity, secret)
func builtinRestBearer(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var tokenEndpoint, identity, secret string
	if err := starlark.UnpackArgs("rest.bearer", args, kwargs,
		"token_endpoint", &tokenEndpoint,
		"identity", &identity,
		"secret", &secret,
	); err != nil {
		return nil, err
	}
	return starlarkAuth{config: rest.BearerAuthConfig{
		TokenEndpoint: tokenEndpoint,
		Identity:      identity,
		Secret:        secret,
	}}, nil
}

// TLS
// -----------------------------------------------------------------------------

func restTLSModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "rest.tls",
		Members: starlark.StringDict{
			"secure":   starlark.NewBuiltin("rest.tls.secure", builtinRestTLSSecure),
			"insecure": starlark.NewBuiltin("rest.tls.insecure", builtinRestTLSInsecure),
			"ca_cert":  starlark.NewBuiltin("rest.tls.ca_cert", builtinRestTLSCACert),
		},
	}
}

type starlarkTLS struct {
	config rest.TLSConfig
}

func (s starlarkTLS) String() string        { return "<rest.tls:" + s.config.Kind() + ">" }
func (s starlarkTLS) Type() string          { return "rest.tls" }
func (s starlarkTLS) Freeze()               {}
func (s starlarkTLS) Truth() starlark.Bool  { return starlark.True }
func (s starlarkTLS) Hash() (uint32, error) { return 0, nil }

func builtinRestTLSSecure(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	if err := starlark.UnpackArgs("rest.tls.secure", args, kwargs); err != nil {
		return nil, err
	}
	return starlarkTLS{config: rest.SecureTLSConfig{}}, nil
}

func builtinRestTLSInsecure(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	if err := starlark.UnpackArgs("rest.tls.insecure", args, kwargs); err != nil {
		return nil, err
	}
	return starlarkTLS{config: rest.InsecureTLSConfig{}}, nil
}

func builtinRestTLSCACert(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackArgs("rest.tls.ca_cert", args, kwargs,
		"path", &path,
	); err != nil {
		return nil, err
	}

	span := callSpan(thread)
	c := threadCollector(thread)

	pem, err := c.src.ReadFile(c.ctx, path)
	if err != nil {
		return nil, &CACertReadError{Path: path, Source: span, Err: err}
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, &CACertParseError{Path: path, Source: span}
	}

	return starlarkTLS{config: rest.CACertTLSConfig{Pool: pool}}, nil
}

// Steps
// -----------------------------------------------------------------------------

// rest.request(method, path, body?, headers?, check?, desc?, on_change?)
func builtinRestRequest(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		method      string
		path        string
		bodyVal     starlark.Value
		headersVal  *starlark.Dict
		checkVal    starlark.Value
		desc        string
		onChangeVal starlark.Value
	)
	if err := starlark.UnpackArgs("rest.request", args, kwargs,
		"method", &method,
		"path", &path,
		"body?", &bodyVal,
		"headers?", &headersVal,
		"check?", &checkVal,
		"desc?", &desc,
		"on_change?", &onChangeVal,
	); err != nil {
		return nil, err
	}

	hookIDs, err := unpackOnChange(thread, onChangeVal, "rest.request")
	if err != nil {
		return nil, err
	}

	var body steprest.BodyConfig
	if bodyVal != nil && bodyVal != starlark.None {
		sb, ok := bodyVal.(starlarkBody)
		if !ok {
			span := callSpan(thread)
			return nil, &TypeError{
				Context:  "rest.request: body",
				Expected: "rest.body.json() or rest.body.string()",
				Got:      bodyVal.Type(),
				Source:   span,
			}
		}
		body = sb.config
	}

	var headers map[string]string
	if headersVal != nil {
		headers = make(map[string]string, headersVal.Len())
		for _, item := range headersVal.Items() {
			k, ok1 := starlark.AsString(item[0])
			v, ok2 := starlark.AsString(item[1])
			if !ok1 || !ok2 {
				return nil, &TypeError{
					Context:  "rest.request: headers",
					Expected: "dict[string, string]",
					Got:      fmt.Sprintf("dict[%s, %s]", item[0].Type(), item[1].Type()),
				}
			}
			headers[k] = v
		}
	}

	var check steprest.CheckConfig
	if checkVal != nil && checkVal != starlark.None {
		sc, ok := checkVal.(starlarkCheck)
		if !ok {
			span := callSpan(thread)
			return nil, &TypeError{
				Context:  "rest.request: check",
				Expected: "rest.status() or rest.jq()",
				Got:      checkVal.Type(),
				Source:   span,
			}
		}
		check = sc.config
	}

	span := callSpan(thread)
	return &StarlarkStep{
		Instance: spec.StepInstance{
			Desc: desc,
			Type: steprest.Request{},
			Config: &steprest.RequestConfig{
				Desc:    desc,
				Method:  method,
				Path:    path,
				Headers: headers,
				Body:    body,
				Check:   check,
			},
			OnChange: hookIDs,
			Source:   span,
			Fields:   kwargsFieldSpans(thread, "method", "path", "body", "headers", "check", "on_change"),
		},
	}, nil
}

// Bodies
// -----------------------------------------------------------------------------

func restBodyModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "rest.body",
		Members: starlark.StringDict{
			"json":   starlark.NewBuiltin("rest.body.json", builtinRestBodyJSON),
			"string": starlark.NewBuiltin("rest.body.string", builtinRestBodyString),
		},
	}
}

type starlarkBody struct {
	config steprest.BodyConfig
}

func (s starlarkBody) String() string        { return "<rest.body:" + s.config.Kind() + ">" }
func (s starlarkBody) Type() string          { return "rest.body" }
func (s starlarkBody) Freeze()               {}
func (s starlarkBody) Truth() starlark.Bool  { return starlark.True }
func (s starlarkBody) Hash() (uint32, error) { return 0, nil }

// rest.body.json(data)
func builtinRestBodyJSON(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var dataVal starlark.Value
	if err := starlark.UnpackArgs("rest.body.json", args, kwargs,
		"data", &dataVal,
	); err != nil {
		return nil, err
	}
	return starlarkBody{config: steprest.JSONBody{Data: starlarkToGo(dataVal)}}, nil
}

// rest.body.string(content)
func builtinRestBodyString(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var content string
	if err := starlark.UnpackArgs("rest.body.string", args, kwargs,
		"content", &content,
	); err != nil {
		return nil, err
	}
	return starlarkBody{config: steprest.StringBody{Content: content}}, nil
}

// Checks
// -----------------------------------------------------------------------------

type starlarkCheck struct {
	config steprest.CheckConfig
}

func (s starlarkCheck) String() string        { return "<rest.check:" + s.config.Kind() + ">" }
func (s starlarkCheck) Type() string          { return "rest.check" }
func (s starlarkCheck) Freeze()               {}
func (s starlarkCheck) Truth() starlark.Bool  { return starlark.True }
func (s starlarkCheck) Hash() (uint32, error) { return 0, nil }

// rest.status(code, path?, method?)
func builtinRestStatus(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		status int
		path   string
		method string
	)
	if err := starlark.UnpackArgs("rest.status", args, kwargs,
		"code", &status,
		"path?", &path,
		"method?", &method,
	); err != nil {
		return nil, err
	}
	return starlarkCheck{config: steprest.StatusCheck{
		Status: status,
		Path:   path,
		Method: method,
	}}, nil
}

// rest.jq(expr, path?, method?)
func builtinRestJQ(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		expr   string
		path   string
		method string
	)
	if err := starlark.UnpackArgs("rest.jq", args, kwargs,
		"expr", &expr,
		"path?", &path,
		"method?", &method,
	); err != nil {
		return nil, err
	}

	compiled, err := steprest.CompileJQ(expr)
	if err != nil {
		span := callSpan(thread)
		return nil, &JQCompileError{Expr: expr, Source: span, Err: err}
	}

	return starlarkCheck{config: &steprest.JQCheck{
		Expr:     expr,
		Path:     path,
		Method:   method,
		Compiled: compiled,
	}}, nil
}

// rest.header(name, value)
func builtinRestHeader(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var name, value string
	if err := starlark.UnpackArgs("rest.header", args, kwargs,
		"name", &name,
		"value", &value,
	); err != nil {
		return nil, err
	}
	return starlarkAuth{config: rest.HeaderAuthConfig{Name: name, Value: value}}, nil
}

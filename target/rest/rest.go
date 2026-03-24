// SPDX-License-Identifier: GPL-3.0-only

package rest

import (
	"context"
	"io"
	"maps"
	"net/http"
	"strings"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

type (
	REST   struct{}
	Config struct {
		BaseURL string
		Auth    AuthConfig
		TLS     TLSConfig
	}
)

func (REST) Kind() string   { return "rest" }
func (REST) NewConfig() any { return &Config{} }
func (REST) Create(_ context.Context, _ source.Source, tgt spec.TargetInstance) (target.Target, error) {
	cfg, ok := tgt.Config.(*Config)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &Config{}, cfg)
	}

	// Normalize base URL: strip trailing slash.
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")

	// Resolve bearer token endpoint relative to base URL before building
	// the transport, so the auth layer has the full URL.
	if bearer, ok := cfg.Auth.(BearerAuthConfig); ok {
		bearer.TokenEndpoint = cfg.BaseURL + bearer.TokenEndpoint
		cfg.Auth = bearer
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = cfg.TLS.TLSClientConfig()
	var base http.RoundTripper = transport
	base = cfg.Auth.Transport(base)

	return &RESTTarget{
		config: cfg,
		client: &http.Client{Transport: base},
	}, nil
}

type RESTTarget struct {
	config *Config
	client *http.Client
}

func (t *RESTTarget) Capabilities() capability.Capability {
	return capability.REST
}

func (t *RESTTarget) Close() {
	t.client.CloseIdleConnections()
}

func (t *RESTTarget) Do(ctx context.Context, req target.HTTPRequest) (*target.HTTPResponse, error) {
	url := t.config.BaseURL + req.Path

	var bodyReader io.Reader
	if req.Body != nil {
		bodyReader = strings.NewReader(string(req.Body))
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, url, bodyReader)
	if err != nil {
		return nil, errs.WrapErrf(errRequest, "build request: %v", err)
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, errs.WrapErrf(errRequest, "%s %s: %v", req.Method, req.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errs.WrapErrf(errRequest, "read response: %v", err)
	}

	headers := make(map[string][]string, len(resp.Header))
	maps.Copy(headers, resp.Header)

	return &target.HTTPResponse{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       body,
	}, nil
}

// bare-error: sentinel for HTTP request failures
var errRequest = errs.New("rest request")

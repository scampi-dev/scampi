// SPDX-License-Identifier: GPL-3.0-only

package testkit

import (
	"context"
	"sync"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

// MockResponse defines a pre-configured HTTP response for a route.
type MockResponse struct {
	StatusCode int
	Headers    map[string][]string
	Body       []byte
}

// RecordedCall captures a single request made through the mock.
type RecordedCall struct {
	Method  string
	Path    string
	Headers map[string]string
	Body    []byte
}

// MockREST implements target.Target and target.HTTPClient against
// a set of pre-configured routes. All requests are recorded for
// assertion verification.
type MockREST struct {
	mu     sync.Mutex
	routes map[string]MockResponse
	calls  []RecordedCall
}

func NewMockREST(routes map[string]MockResponse) *MockREST {
	return &MockREST{routes: routes}
}

func (m *MockREST) Capabilities() capability.Capability {
	return capability.REST
}

func (m *MockREST) Do(
	_ context.Context,
	req target.HTTPRequest,
) (*target.HTTPResponse, error) {
	m.mu.Lock()
	m.calls = append(m.calls, RecordedCall{
		Method:  req.Method,
		Path:    req.Path,
		Headers: req.Headers,
		Body:    req.Body,
	})
	m.mu.Unlock()

	key := req.Method + " " + req.Path
	if resp, ok := m.routes[key]; ok {
		headers := make(map[string][]string, len(resp.Headers))
		for k, v := range resp.Headers {
			headers[k] = v
		}
		return &target.HTTPResponse{
			StatusCode: resp.StatusCode,
			Headers:    headers,
			Body:       resp.Body,
		}, nil
	}

	return &target.HTTPResponse{
		StatusCode: 404,
		Body:       []byte(`{"error":"no route for ` + key + `"}`),
	}, nil
}

// Calls returns a snapshot of all recorded requests.
func (m *MockREST) Calls() []RecordedCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RecordedCall, len(m.calls))
	copy(out, m.calls)
	return out
}

// CallsMatching returns recorded requests matching method and path.
func (m *MockREST) CallsMatching(method, path string) []RecordedCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []RecordedCall
	for _, c := range m.calls {
		if c.Method == method && c.Path == path {
			out = append(out, c)
		}
	}
	return out
}

// RESTMockTargetType wraps a pre-built MockREST as a spec.TargetType.
type RESTMockTargetType struct {
	Tgt *MockREST
}

func (t RESTMockTargetType) Kind() string   { return "rest_mock" }
func (t RESTMockTargetType) NewConfig() any { return nil }

func (t RESTMockTargetType) Create(
	_ context.Context,
	_ source.Source,
	_ spec.TargetInstance,
) (target.Target, error) {
	return t.Tgt, nil
}

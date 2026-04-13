// SPDX-License-Identifier: GPL-3.0-only

package testkit

import (
	"context"
	"strings"
	"testing"

	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/target"
)

func makeRESTMock(routes map[string]target.MemRESTResponse, calls ...target.HTTPRequest) *target.MemREST {
	mock := target.NewMemREST(routes)
	for _, req := range calls {
		_, _ = mock.Do(context.Background(), req)
	}
	return mock
}

func requestMatcher(
	method, path string,
	extras map[string]eval.Value,
) *eval.StructVal {
	fields := map[string]eval.Value{
		"method": &eval.StringVal{V: method},
		"path":   &eval.StringVal{V: path},
	}
	for k, v := range extras {
		fields[k] = v
	}
	return &eval.StructVal{
		TypeName: "request",
		QualName: "test.request",
		RetType:  "RequestMatcher",
		Fields:   fields,
	}
}

func expectRequests(matchers ...*eval.StructVal) *eval.ListVal {
	items := make([]eval.Value, len(matchers))
	for i, m := range matchers {
		items[i] = m
	}
	return &eval.ListVal{Items: items}
}

func TestVerifyMemREST_CalledOnce(t *testing.T) {
	mock := makeRESTMock(nil,
		target.HTTPRequest{Method: "POST", Path: "/items"},
	)
	expect := expectRequests(
		requestMatcher("POST", "/items", nil),
	)
	if got := VerifyMemREST(expect, mock); len(got) != 0 {
		t.Errorf("expected clean verify, got: %+v", got)
	}
}

func TestVerifyMemREST_NotCalled(t *testing.T) {
	mock := makeRESTMock(nil) // no calls
	expect := expectRequests(
		requestMatcher("POST", "/items", nil),
	)
	got := VerifyMemREST(expect, mock)
	if len(got) != 1 {
		t.Fatalf("expected 1 mismatch, got %d", len(got))
	}
	if !strings.Contains(got[0].Reason, "expected at least one") {
		t.Errorf("reason: %q", got[0].Reason)
	}
}

func TestVerifyMemREST_ExactCount(t *testing.T) {
	mock := makeRESTMock(nil,
		target.HTTPRequest{Method: "GET", Path: "/x"},
		target.HTTPRequest{Method: "GET", Path: "/x"},
	)
	// Expect exactly 2.
	expect := expectRequests(requestMatcher(
		"GET",
		"/x",
		map[string]eval.Value{"count": &eval.IntVal{V: 2}},
	))
	if got := VerifyMemREST(expect, mock); len(got) != 0 {
		t.Errorf("expected clean verify, got: %+v", got)
	}

	// Expect exactly 3 — should fail.
	expect = expectRequests(requestMatcher(
		"GET",
		"/x",
		map[string]eval.Value{"count": &eval.IntVal{V: 3}},
	))
	got := VerifyMemREST(expect, mock)
	if len(got) != 1 {
		t.Fatalf("expected 1 mismatch, got %d", len(got))
	}
	if !strings.Contains(got[0].Reason, "exactly 3") {
		t.Errorf("reason: %q", got[0].Reason)
	}
}

func TestVerifyMemREST_CountAtLeast(t *testing.T) {
	mock := makeRESTMock(nil,
		target.HTTPRequest{Method: "POST", Path: "/y"},
		target.HTTPRequest{Method: "POST", Path: "/y"},
	)
	// At least 2 — should pass.
	expect := expectRequests(requestMatcher(
		"POST",
		"/y",
		map[string]eval.Value{"count_at_least": &eval.IntVal{V: 2}},
	))
	if got := VerifyMemREST(expect, mock); len(got) != 0 {
		t.Errorf("expected clean verify, got: %+v", got)
	}

	// At least 3 — should fail.
	expect = expectRequests(requestMatcher(
		"POST",
		"/y",
		map[string]eval.Value{"count_at_least": &eval.IntVal{V: 3}},
	))
	got := VerifyMemREST(expect, mock)
	if len(got) != 1 {
		t.Fatalf("expected 1 mismatch, got %d", len(got))
	}
	if !strings.Contains(got[0].Reason, "at least 3") {
		t.Errorf("reason: %q", got[0].Reason)
	}
}

func TestVerifyMemREST_BodyMatcher(t *testing.T) {
	mock := makeRESTMock(nil,
		target.HTTPRequest{
			Method: "POST",
			Path:   "/items",
			Body:   []byte(`{"name":"hello"}`),
		},
	)
	// Body contains "hello" — pass.
	expect := expectRequests(requestMatcher(
		"POST", "/items",
		map[string]eval.Value{
			"body": matcher("has_substring", map[string]string{"substring": "hello"}),
		},
	))
	if got := VerifyMemREST(expect, mock); len(got) != 0 {
		t.Errorf("expected clean verify, got: %+v", got)
	}

	// Body contains "missing" — fail.
	expect = expectRequests(requestMatcher(
		"POST", "/items",
		map[string]eval.Value{
			"body": matcher("has_substring", map[string]string{"substring": "missing"}),
		},
	))
	got := VerifyMemREST(expect, mock)
	if len(got) != 1 {
		t.Fatalf("expected 1 mismatch, got %d", len(got))
	}
	if !strings.Contains(got[0].Reason, "missing substring") {
		t.Errorf("reason: %q", got[0].Reason)
	}
}

func TestVerifyMemREST_NilInputs(t *testing.T) {
	if got := VerifyMemREST(nil, target.NewMemREST(nil)); got != nil {
		t.Errorf("nil expect: got %+v", got)
	}
	if got := VerifyMemREST(expectRequests(), nil); got != nil {
		t.Errorf("nil mock: got %+v", got)
	}
}

func TestVerifyMemREST_MultipleMatchers(t *testing.T) {
	mock := makeRESTMock(nil,
		target.HTTPRequest{Method: "POST", Path: "/a"},
		target.HTTPRequest{Method: "DELETE", Path: "/b"},
	)
	expect := expectRequests(
		requestMatcher("POST", "/a", nil),
		requestMatcher("DELETE", "/b", nil),
		requestMatcher("GET", "/missing", nil), // should fail
	)
	got := VerifyMemREST(expect, mock)
	if len(got) != 1 {
		t.Fatalf("expected 1 mismatch, got %d: %+v", len(got), got)
	}
	if got[0].Key != "GET /missing" {
		t.Errorf("key: %q", got[0].Key)
	}
}

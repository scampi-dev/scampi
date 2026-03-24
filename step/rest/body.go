// SPDX-License-Identifier: GPL-3.0-only

package rest

import "encoding/json"

// BodyConfig prepares a request body and its associated headers.
// Implementations are constructed in Starlark (rest.body.json, rest.body.string)
// and stored in RequestConfig.Body.
type BodyConfig interface {
	Bytes() ([]byte, error)
	Headers() map[string]string
	Kind() string
}

// JSONBody
// -----------------------------------------------------------------------------

type JSONBody struct {
	Data any
}

func (JSONBody) Kind() string { return "json" }

func (b JSONBody) Bytes() ([]byte, error) {
	return json.Marshal(b.Data)
}

func (JSONBody) Headers() map[string]string {
	return map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}
}

// StringBody
// -----------------------------------------------------------------------------

type StringBody struct {
	Content string
}

func (StringBody) Kind() string { return "string" }

func (b StringBody) Bytes() ([]byte, error) {
	return []byte(b.Content), nil
}

func (StringBody) Headers() map[string]string { return nil }

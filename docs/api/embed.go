// Package docsapi embeds the OpenAPI specification for the mingyue agent API.
package docsapi

import _ "embed"

// OpenAPISpec contains the raw bytes of the OpenAPI 3.0 YAML specification.
//
//go:embed openapi.yaml
var OpenAPISpec []byte

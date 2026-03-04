package api

import (
	"net/http"

	docsapi "kopelan/mingyue-go/docs/api"
)

// swaggerUIHTML is the HTML page served at /swagger/ that loads the Swagger UI
// from CDN and points it at the embedded OpenAPI spec.
// CDN resources are pinned to a specific version with SRI hashes for security.
const swaggerUIHTML = `<!DOCTYPE html>
<html>
<head>
  <title>mingyue API – Swagger UI</title>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <link rel="stylesheet" type="text/css"
        href="https://unpkg.com/swagger-ui-dist@5.32.0/swagger-ui.css"
        integrity="sha384-3nuX7df3EaAoiqLBeyS1Ola0Gpg9ryJKVtarubwfnA1cOH8AWHUdbPSIvEqPZ9VH"
        crossorigin="anonymous">
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5.32.0/swagger-ui-bundle.js"
        integrity="sha384-7xcoc6ZKDFF7Ek627QTC3Bg/K+5Y36NJ8MWAE43D2m6+3Sh9XO3tdsfHhrS8gNIQ"
        crossorigin="anonymous"></script>
<script>
  window.onload = function() {
    SwaggerUIBundle({
      url: "/swagger/openapi.yaml",
      dom_id: '#swagger-ui',
      presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
      layout: "BaseLayout"
    });
  };
</script>
</body>
</html>
`

// swaggerUIHandler serves the Swagger UI HTML page at GET /swagger/.
// Any path other than "/swagger/" or "/swagger" returns 404 to avoid
// accidentally serving the UI page for unrecognised sub-paths.
func swaggerUIHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/swagger/" && r.URL.Path != "/swagger" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(swaggerUIHTML))
}

// swaggerSpecHandler serves the embedded OpenAPI YAML spec at GET /swagger/openapi.yaml.
func swaggerSpecHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(docsapi.OpenAPISpec)
}

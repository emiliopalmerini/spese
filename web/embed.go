package web

import "embed"

// TemplatesFS embeds HTML templates for server-side rendering.
//go:embed templates/*.html
var TemplatesFS embed.FS


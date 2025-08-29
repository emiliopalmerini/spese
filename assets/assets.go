package assets

import "embed"

// TemplatesFS embeds HTML templates for server-side rendering.
//go:embed web/templates/*.html
var TemplatesFS embed.FS


package web

import "embed"

// TemplatesFS embeds HTML templates for server-side rendering.
//
//go:embed all:templates
var TemplatesFS embed.FS

// StaticFS embeds static assets (css/js/images).
//
//go:embed static
var StaticFS embed.FS

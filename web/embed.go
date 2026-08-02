package web

import "embed"

// AppFS contains the hashed Vite build served by the Go runtime.
//
//go:embed all:dist
var AppFS embed.FS

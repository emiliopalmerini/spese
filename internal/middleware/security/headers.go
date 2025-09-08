package security

import (
	"fmt"
	"net/http"
)

// HeadersConfig holds security headers configuration
type HeadersConfig struct {
	// Content Security Policy
	CSP string
	
	// HSTS settings
	HSTSMaxAge            int
	HSTSIncludeSubdomains bool
	HSTSPreload           bool
	
	// Additional security headers
	XFrameOptions        string
	XContentTypeOptions  string
	XXSSProtection       string
	ReferrerPolicy       string
	PermissionsPolicy    string
	CrossOriginOpener    string
	CrossOriginEmbedder  string
	CrossOriginResource  string
}

// DefaultHeadersConfig returns secure defaults
func DefaultHeadersConfig() HeadersConfig {
	return HeadersConfig{
		CSP: "default-src 'self'; " +
			"script-src 'self' https://unpkg.com; " +
			"style-src 'self' 'unsafe-inline'; " +
			"img-src 'self' data:; " +
			"connect-src 'self'; " +
			"font-src 'self'; " +
			"object-src 'none'; " +
			"media-src 'self'; " +
			"frame-ancestors 'none'; " +
			"base-uri 'self'; " +
			"form-action 'self'",
		
		HSTSMaxAge:            31536000, // 1 year
		HSTSIncludeSubdomains: true,
		HSTSPreload:           true,
		
		XFrameOptions:       "DENY",
		XContentTypeOptions: "nosniff",
		XXSSProtection:      "1; mode=block",
		ReferrerPolicy:      "strict-origin-when-cross-origin",
		PermissionsPolicy:   "geolocation=(), microphone=(), camera=(), payment=()",
		CrossOriginOpener:   "same-origin",
		CrossOriginEmbedder: "require-corp",
		CrossOriginResource: "same-origin",
	}
}

// HeadersMiddleware applies security headers to responses
type HeadersMiddleware struct {
	config HeadersConfig
}

// NewHeadersMiddleware creates a new security headers middleware
func NewHeadersMiddleware(config HeadersConfig) *HeadersMiddleware {
	return &HeadersMiddleware{
		config: config,
	}
}

// Middleware returns the HTTP middleware function
func (h *HeadersMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Apply security headers
		h.applyHeaders(w, r)
		
		next.ServeHTTP(w, r)
	})
}

func (h *HeadersMiddleware) applyHeaders(w http.ResponseWriter, r *http.Request) {
	headers := w.Header()
	
	// Basic security headers
	headers.Set("X-Content-Type-Options", h.config.XContentTypeOptions)
	headers.Set("X-Frame-Options", h.config.XFrameOptions)
	headers.Set("X-XSS-Protection", h.config.XXSSProtection)
	
	// Content Security Policy
	if h.config.CSP != "" {
		headers.Set("Content-Security-Policy", h.config.CSP)
	}
	
	// Modern security headers
	headers.Set("Referrer-Policy", h.config.ReferrerPolicy)
	headers.Set("Permissions-Policy", h.config.PermissionsPolicy)
	headers.Set("Cross-Origin-Opener-Policy", h.config.CrossOriginOpener)
	headers.Set("Cross-Origin-Embedder-Policy", h.config.CrossOriginEmbedder)
	headers.Set("Cross-Origin-Resource-Policy", h.config.CrossOriginResource)
	
	// HSTS header (only for HTTPS)
	if r.TLS != nil && h.config.HSTSMaxAge > 0 {
		hstsValue := fmt.Sprintf("max-age=%d", h.config.HSTSMaxAge)
		if h.config.HSTSIncludeSubdomains {
			hstsValue += "; includeSubDomains"
		}
		if h.config.HSTSPreload {
			hstsValue += "; preload"
		}
		headers.Set("Strict-Transport-Security", hstsValue)
	}
}

// StaticAssetMiddleware adds caching headers for static assets
func StaticAssetMiddleware(maxAge int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if maxAge > 0 {
				w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d, immutable", maxAge))
			}
			next.ServeHTTP(w, r)
		})
	}
}


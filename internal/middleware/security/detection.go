package security

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
)

// DetectionMetrics tracks security detection events
type DetectionMetrics struct {
	SuspiciousRequests int64
	InvalidIPAttempts  int64
}

// Detector handles suspicious request detection
type Detector struct {
	metrics       *DetectionMetrics
	trustedProxies []*net.IPNet
}

// NewDetector creates a new security detector
func NewDetector() *Detector {
	return &Detector{
		metrics: &DetectionMetrics{},
		trustedProxies: []*net.IPNet{
			parseCIDR("127.0.0.0/8"),   // localhost
			parseCIDR("10.0.0.0/8"),    // private networks
			parseCIDR("172.16.0.0/12"), // private networks  
			parseCIDR("192.168.0.0/16"), // private networks
		},
	}
}

// parseCIDR is a helper to parse CIDR during initialization
func parseCIDR(cidr string) *net.IPNet {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(fmt.Sprintf("failed to parse trusted proxy CIDR %s: %v", cidr, err))
	}
	return network
}

// DetectSuspiciousRequest analyzes request patterns for potential threats
func (d *Detector) DetectSuspiciousRequest(r *http.Request) bool {
	suspicious := false
	
	// Check for common attack patterns in URL path
	path := strings.ToLower(r.URL.Path)
	suspiciousPatterns := []string{
		"../", "..\\", ".env", "wp-admin", "phpmyadmin", 
		"admin.php", "config.php", ".git", ".ssh",
		"eval(", "javascript:", "<script", "union select",
		"base64", "0x", "etc/passwd", "cmd.exe",
	}
	
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(path, pattern) {
			suspicious = true
			break
		}
	}
	
	// Check for suspicious query parameters
	query := strings.ToLower(r.URL.RawQuery)
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(query, pattern) {
			suspicious = true
			break
		}
	}
	
	// Check User-Agent for common bot patterns
	userAgent := strings.ToLower(r.Header.Get("User-Agent"))
	suspiciousAgents := []string{
		"sqlmap", "nmap", "nikto", "gobuster", "dirb", 
		"curl", "wget", "python-requests", "scanner",
		"bot", "crawler", "spider", "scraper",
	}
	
	for _, agent := range suspiciousAgents {
		if strings.Contains(userAgent, agent) {
			suspicious = true
			break
		}
	}
	
	// Check for unusual HTTP methods
	unusualMethods := []string{"TRACE", "TRACK", "DEBUG", "CONNECT"}
	for _, method := range unusualMethods {
		if r.Method == method {
			suspicious = true
			break
		}
	}
	
	// Check for excessively long URLs (possible overflow attempt)
	if len(r.URL.String()) > 2048 {
		suspicious = true
	}
	
	// Check for suspicious headers
	if r.Header.Get("X-Forwarded-For") != "" && r.Header.Get("X-Real-IP") != "" {
		// Multiple forwarding headers might indicate header manipulation
		xff := r.Header.Get("X-Forwarded-For")
		if strings.Count(xff, ",") > 5 { // More than 5 proxy hops is suspicious
			suspicious = true
		}
	}
	
	if suspicious {
		atomic.AddInt64(&d.metrics.SuspiciousRequests, 1)
	}
	
	return suspicious
}

// ExtractClientIP extracts the real client IP, validating forwarded headers
func (d *Detector) ExtractClientIP(r *http.Request) string {
	// Start with the direct connection IP
	directIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// If parsing fails, use RemoteAddr as-is (fallback)
		directIP = r.RemoteAddr
	}
	
	parsedDirectIP := net.ParseIP(directIP)
	if parsedDirectIP == nil {
		return directIP // Fallback to original if parsing fails
	}
	
	// If direct connection is from trusted proxy, check forwarded headers
	if d.isTrustedProxy(parsedDirectIP) {
		// Check X-Forwarded-For header (most common)
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// X-Forwarded-For can contain multiple IPs, take the first one
			ips := strings.Split(xff, ",")
			if len(ips) > 0 {
				clientIP := strings.TrimSpace(ips[0])
				if parsedIP := net.ParseIP(clientIP); parsedIP != nil {
					return clientIP
				}
			}
		}
		
		// Check X-Real-IP header (nginx)
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			if parsedIP := net.ParseIP(xri); parsedIP != nil {
				return xri
			}
		}
	}
	
	// Return direct IP if no valid forwarded IP found
	return directIP
}

// isTrustedProxy checks if an IP is from a trusted proxy
func (d *Detector) isTrustedProxy(ip net.IP) bool {
	for _, network := range d.trustedProxies {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// GetMetrics returns current security metrics
func (d *Detector) GetMetrics() DetectionMetrics {
	return DetectionMetrics{
		SuspiciousRequests: atomic.LoadInt64(&d.metrics.SuspiciousRequests),
		InvalidIPAttempts:  atomic.LoadInt64(&d.metrics.InvalidIPAttempts),
	}
}

// AddTrustedProxy adds a trusted proxy network
func (d *Detector) AddTrustedProxy(cidr string) error {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR %s: %w", cidr, err)
	}
	
	d.trustedProxies = append(d.trustedProxies, network)
	return nil
}
package http

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
)

// securityMetrics tracks security-related events.
type securityMetrics struct {
	rateLimitHits      int64
	invalidIPAttempts  int64
	suspiciousRequests int64
}

// trustedProxies defines networks that are trusted to set forwarding headers.
var trustedProxies = []*net.IPNet{
	parsecidr("127.0.0.0/8"),    // localhost
	parsecidr("10.0.0.0/8"),     // private networks
	parsecidr("172.16.0.0/12"),  // private networks
	parsecidr("192.168.0.0/16"), // private networks
}

// parsecidr is a helper to parse CIDR during initialization.
func parsecidr(cidr string) *net.IPNet {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(fmt.Sprintf("failed to parse trusted proxy CIDR %s: %v", cidr, err))
	}
	return network
}

// isTrustedProxy checks if an IP is from a trusted proxy.
func isTrustedProxy(ip net.IP) bool {
	for _, network := range trustedProxies {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// extractClientIP extracts the real client IP, validating forwarded headers.
func extractClientIP(r *http.Request) string {
	directIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		directIP = r.RemoteAddr
	}

	parsedDirectIP := net.ParseIP(directIP)
	if parsedDirectIP == nil {
		return directIP
	}

	if isTrustedProxy(parsedDirectIP) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ips := strings.Split(xff, ",")
			if len(ips) > 0 {
				clientIP := strings.TrimSpace(ips[0])
				if parsedIP := net.ParseIP(clientIP); parsedIP != nil {
					return clientIP
				}
			}
		}

		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			if parsedIP := net.ParseIP(xri); parsedIP != nil {
				return xri
			}
		}
	}

	return directIP
}

// detectSuspiciousRequest analyzes request patterns for potential threats.
func detectSuspiciousRequest(r *http.Request, metrics *securityMetrics) bool {
	suspicious := false

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

	query := strings.ToLower(r.URL.RawQuery)
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(query, pattern) {
			suspicious = true
			break
		}
	}

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

	unusualMethods := []string{"TRACE", "TRACK", "DEBUG", "CONNECT"}
	for _, method := range unusualMethods {
		if r.Method == method {
			suspicious = true
			break
		}
	}

	if len(r.URL.String()) > 2048 {
		suspicious = true
	}

	if r.Header.Get("X-Forwarded-For") != "" && r.Header.Get("X-Real-IP") != "" {
		xff := r.Header.Get("X-Forwarded-For")
		if strings.Count(xff, ",") > 5 {
			suspicious = true
		}
	}

	if suspicious && metrics != nil {
		atomic.AddInt64(&metrics.suspiciousRequests, 1)
	}

	return suspicious
}

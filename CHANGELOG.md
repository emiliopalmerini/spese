# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Graceful shutdown handling with proper signal handling
- Comprehensive rate limiting (60 requests/minute per IP)  
- Security headers: CSP, XSS Protection, X-Frame-Options, X-Content-Type-Options
- Input sanitization for all user inputs
- Server timeouts and request size limits
- Enhanced error handling with proper logging
- Docker-based OAuth initialization flow
- Comprehensive golangci-lint configuration
- Improved pre-commit hooks with additional checks

### Fixed
- Integer overflow protection in money parsing
- Race conditions in main server loop
- Inconsistent error handling across HTTP handlers
- Memory leaks in rate limiter (periodic cleanup needed)
- Docker compose default sheet names consistency
- OAuth token file permissions (now 0600)
- Template parsing errors with better fallbacks

### Security
- Input validation with length limits (descriptions max 200 chars)
- Control character filtering in user inputs
- Proper HTML escaping in all templates
- Rate limiting to prevent abuse
- Secure OAuth token storage
- Removed hardcoded test spreadsheet ID

### Changed
- Server now requires explicit shutdown signal
- HTTP handlers return proper status codes and headers
- Improved error messages with more context
- Updated ADR 0001 to reflect OAuth-only architecture
- Enhanced documentation with security notes

### Performance
- Added connection timeouts and limits
- Improved template caching
- Better memory management in rate limiting
- Optimized static file serving with cache headers

## [Previous Versions]

See git history for earlier changes.
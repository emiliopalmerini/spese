package log

// Common field names for structured logging
const (
	FieldComponent     = "component"
	FieldRequestID     = "request_id"
	FieldClientIP      = "client_ip"
	FieldMethod        = "method"
	FieldPath          = "path"
	FieldQuery         = "query"
	FieldStatusCode    = "status_code"
	FieldDuration      = "duration_ms"
	FieldDurationHuman = "duration_human"
	FieldUserAgent     = "user_agent"
	FieldReferer       = "referer"
	FieldSuccess       = "success"
	FieldError         = "error"
	FieldOperation     = "operation"
	FieldYear          = "year"
	FieldMonth         = "month"
	FieldExpenseDesc   = "expense_description"
	FieldAmountCents   = "amount_cents"
	FieldPrimaryCategory   = "primary_category"
	FieldSecondaryCategory = "secondary_category"
	FieldSheetsRef     = "sheets_ref"
)

// Components defines standard component names
const (
	ComponentApp         = "app"
	ComponentHTTP        = "http"
	ComponentExpense     = "expense"
	ComponentStorage     = "storage"
	ComponentAMQP        = "amqp"
	ComponentWorker      = "worker"
	ComponentSheets      = "sheets"
	ComponentCache       = "cache"
	ComponentSecurity    = "security"
	ComponentRateLimit   = "rate_limit"
	ComponentTrace       = "trace"
	ComponentBackend     = "backend"
	ComponentTemplate    = "template"
)

// Operations defines standard operation names  
const (
	OpCreate   = "create"
	OpRead     = "read" 
	OpUpdate   = "update"
	OpDelete   = "delete"
	OpList     = "list"
	OpAppend   = "append"
	OpSync     = "sync"
	OpValidate = "validate"
	OpParse    = "parse"
	OpRender   = "render"
	OpShutdown = "shutdown"
	OpStartup  = "startup"
)

// ErrorTypes defines standard error type categories
const (
	ErrorTypeValidation     = "validation_error"
	ErrorTypeConfiguration = "configuration_error"
	ErrorTypeDatabase      = "database_error"
	ErrorTypeNetwork       = "network_error"
	ErrorTypeAuth          = "auth_error"
	ErrorTypeTimeout       = "timeout_error"
	ErrorTypeNotFound      = "not_found_error"
	ErrorTypeConflict      = "conflict_error"
	ErrorTypeInternal      = "internal_error"
)

// LogFields provides a builder pattern for structured log fields
type LogFields map[string]any

// NewFields creates a new LogFields instance
func NewFields() LogFields {
	return make(LogFields)
}

// WithComponent adds component field
func (f LogFields) WithComponent(component string) LogFields {
	f[FieldComponent] = component
	return f
}

// WithRequestID adds request ID field
func (f LogFields) WithRequestID(requestID string) LogFields {
	f[FieldRequestID] = requestID
	return f
}

// WithClientIP adds client IP field
func (f LogFields) WithClientIP(ip string) LogFields {
	f[FieldClientIP] = ip
	return f
}

// WithError adds error field
func (f LogFields) WithError(err error) LogFields {
	if err != nil {
		f[FieldError] = err.Error()
	}
	return f
}

// WithOperation adds operation field
func (f LogFields) WithOperation(op string) LogFields {
	f[FieldOperation] = op
	return f
}

// WithExpense adds expense-related fields
func (f LogFields) WithExpense(desc string, amountCents int64, primary, secondary string) LogFields {
	f[FieldExpenseDesc] = desc
	f[FieldAmountCents] = amountCents
	f[FieldPrimaryCategory] = primary
	f[FieldSecondaryCategory] = secondary
	return f
}

// WithHTTPRequest adds HTTP request fields
func (f LogFields) WithHTTPRequest(method, path, query, userAgent, referer string) LogFields {
	f[FieldMethod] = method
	f[FieldPath] = path
	f[FieldQuery] = query
	f[FieldUserAgent] = userAgent
	f[FieldReferer] = referer
	return f
}

// WithHTTPResponse adds HTTP response fields  
func (f LogFields) WithHTTPResponse(statusCode int, durationMs int64, success bool) LogFields {
	f[FieldStatusCode] = statusCode
	f[FieldDuration] = durationMs
	f[FieldSuccess] = success
	return f
}

// ToSlice converts LogFields to a slice for slog
func (f LogFields) ToSlice() []any {
	slice := make([]any, 0, len(f)*2)
	for k, v := range f {
		slice = append(slice, k, v)
	}
	return slice
}
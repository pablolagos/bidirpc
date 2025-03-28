package bidirpc

import "strconv"

// Context provides helper methods to access RPC request parameters and send responses.
type Context struct {
	params    map[string]any
	conn      *Connection
	requestID string
}

// GetParamString returns the string parameter for the given name, or def if not present.
func (ctx *Context) GetParamString(name, def string) string {
	if val, ok := ctx.params[name]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return def
}

// GetParamInt returns the integer parameter for the given name, or def if not present.
func (ctx *Context) GetParamInt(name string, def int) int {
	if val, ok := ctx.params[name]; ok {
		switch v := val.(type) {
		case float64:
			return int(v)
		case int:
			return v
		case string:
			if i, err := strconv.Atoi(v); err == nil {
				return i
			}
		}
	}
	return def
}

// GetParamFloat returns the float parameter for the given name, or def if not present.
func (ctx *Context) GetParamFloat(name string, def float64) float64 {
	if val, ok := ctx.params[name]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f
			}
		}
	}
	return def
}

// WriteResponse sends a successful RPC response with the provided result.
func (ctx *Context) WriteResponse(response any) {
	msg := RPCMessage{
		Type:   ResponseType,
		ID:     ctx.requestID,
		Result: response,
	}
	ctx.conn.Send(msg)
}

// WriteError sends an RPC error response with the given code and message.
func (ctx *Context) WriteError(code int, message string) {
	msg := RPCMessage{
		Type:      ResponseType,
		ID:        ctx.requestID,
		Error:     &message,
		ErrorCode: code,
	}
	ctx.conn.Send(msg)
}

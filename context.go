package bidirpc

type Context struct {
	conn     *Connection
	clientID string
	id       string
	params   map[string]any
}

// ClientID returns the ID of the client that sent the current request.
func (ctx *Context) ClientID() string {
	return ctx.clientID
}

// GetParamString retrieves a string parameter with default value.
func (ctx *Context) GetParamString(name, def string) string {
	val, ok := ctx.params[name]
	if !ok {
		return def
	}
	str, ok := val.(string)
	if !ok {
		return def
	}
	return str
}

// GetParamInt retrieves an int parameter with default value.
func (ctx *Context) GetParamInt(name string, def int) int {
	val, ok := ctx.params[name]
	if !ok {
		return def
	}
	f, ok := val.(float64) // JSON numbers are float64
	if !ok {
		return def
	}
	return int(f)
}

// WriteResponse sends a successful response back to the caller.
func (ctx *Context) WriteResponse(result any) {
	msg := RPCMessage{
		Type:   ResponseType,
		ID:     ctx.id,
		Result: result,
	}
	_ = ctx.conn.Send(msg)
}

// WriteError sends an error response back to the caller.
func (ctx *Context) WriteError(code int, message string) {
	msg := RPCMessage{
		Type:      ResponseType,
		ID:        ctx.id,
		Error:     &message,
		ErrorCode: code,
	}
	_ = ctx.conn.Send(msg)
}

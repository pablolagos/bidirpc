package bidirpc

type HandlerFunc func(ctx *Context)

type HandlerRegistry struct {
	handlers map[string]HandlerFunc
}

func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{
		handlers: make(map[string]HandlerFunc),
	}
}

func (r *HandlerRegistry) Register(method string, fn HandlerFunc) {
	r.handlers[method] = fn
}

func (r *HandlerRegistry) Handle(conn *Connection, msg RPCMessage) {
	fn, ok := r.handlers[msg.Method]
	if !ok {
		conn.Send(RPCMessage{
			Type:      ResponseType,
			ID:        msg.ID,
			Error:     strPtr("method not found"),
			ErrorCode: 404,
		})
		return
	}

	ctx := &Context{
		conn:      conn,
		requestID: msg.ID,
		params:    msg.Params,
	}
	fn(ctx)
}

func strPtr(s string) *string {
	return &s
}

package bidirpc

import (
	"sync"
)

type HandlerFunc func(ctx *Context)

type HandlerRegistry struct {
	handlers map[string]HandlerFunc
	mu       sync.RWMutex
}

func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{
		handlers: make(map[string]HandlerFunc),
	}
}

func (hr *HandlerRegistry) Register(method string, fn HandlerFunc) {
	hr.handlers[method] = fn
}

func (hr *HandlerRegistry) Handle(conn *Connection, msg RPCMessage) {
	fn, ok := hr.handlers[msg.Method]
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
		conn:   conn,
		id:     msg.ID,
		params: msg.Params,
	}
	fn(ctx)
}

func (r *HandlerRegistry) HandleWithContext(ctx *Context, method string) {
	fn := r.Get(method)
	if fn != nil {
		fn(ctx)
	} else {
		ctx.WriteError(404, "method not found")
	}
}

func strPtr(s string) *string {
	return &s
}

func (hr *HandlerRegistry) Get(method string) HandlerFunc {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	return hr.handlers[method]
}

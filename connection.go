package bidirpc

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Connection struct {
	Conn           net.Conn
	Enc            *json.Encoder
	Dec            *json.Decoder
	sendMu         sync.Mutex // protects Send()
	initMu         sync.Mutex // protects gzip.Reader setup and decoder init
	useCompression bool
	gzWriter       *gzip.Writer
	gzReader       *gzip.Reader
	pending        map[string]chan RPCMessage
	pendingMu      sync.Mutex
	handlers       *HandlerRegistry
	clientID       string
}

func NewConnection(conn net.Conn) *Connection {
	return &Connection{
		Conn:     conn,
		Enc:      json.NewEncoder(conn),
		Dec:      json.NewDecoder(conn),
		pending:  make(map[string]chan RPCMessage),
		handlers: NewHandlerRegistry(),
	}
}

type ResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (r *ResponseError) Error() string {
	return fmt.Sprintf("error %d: %s", r.Code, r.Message)
}

// EnableCompression sets up gzip writer and marks the connection as compressed.
// Reader is initialized lazily in readLoop.
func (c *Connection) EnableCompression() error {
	c.initMu.Lock()
	defer c.initMu.Unlock()

	if c.useCompression {
		return nil
	}

	c.gzWriter = gzip.NewWriter(c.Conn)
	c.Enc = json.NewEncoder(c.gzWriter)
	c.useCompression = true
	return nil
}

// StartReadLoop begins the message decoding loop in a goroutine.
func (c *Connection) StartReadLoop() {
	go c.readLoop()
}

func (c *Connection) readLoop() {
	defer func() {
		log.Println("[conn] readLoop terminated")
		c.connClosed()
	}()

	for {
		c.initMu.Lock()
		if c.useCompression && c.gzReader == nil {
			gr, err := gzip.NewReader(c.Conn)
			if err != nil {
				c.initMu.Unlock()
				log.Println("[conn] gzip.NewReader failed:", err)
				return
			}
			c.gzReader = gr
			c.Dec = json.NewDecoder(gr)
		}
		dec := c.Dec
		c.initMu.Unlock()

		var msg RPCMessage
		if err := dec.Decode(&msg); err != nil {
			log.Println("[conn] decode error:", err)
			return
		}
		c.handleMessage(msg)
	}
}

func (c *Connection) connClosed() {
	// Placeholder for optional client/server cleanup callback
}

// Send serializes and transmits a message. Safe for concurrent use.
func (c *Connection) Send(msg RPCMessage) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	if err := c.Enc.Encode(msg); err != nil {
		return err
	}
	if c.useCompression && c.gzWriter != nil {
		return c.gzWriter.Flush()
	}
	return nil
}

func (c *Connection) handleMessage(msg RPCMessage) {
	switch msg.Type {
	case ResponseType:
		c.pendingMu.Lock()
		ch, ok := c.pending[msg.ID]
		if ok {
			ch <- msg
			delete(c.pending, msg.ID)
		}
		c.pendingMu.Unlock()

	case RequestType:
		ctx := &Context{
			conn:     c,
			clientID: c.clientID,
			id:       msg.ID,
			params:   msg.Params,
		}
		fn := c.handlers.Get(msg.Method)
		if fn != nil {
			go fn(ctx)
		} else {
			ctx.WriteError(404, "method not found")
		}

	default:
		log.Println("[conn] unknown message type:", msg.Type)
	}
}

// Call sends a request and waits for a response.
func (c *Connection) Call(method string, params map[string]any, timeout time.Duration) (any, error) {
	id := uuid.NewString()
	ch := make(chan RPCMessage, 1)

	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	req := RPCMessage{
		Type:   RequestType,
		ID:     id,
		Method: method,
		Params: params,
	}

	if err := c.Send(req); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}

	select {
	case msg := <-ch:
		if msg.Error != nil {
			return nil, &ResponseError{
				Code:    msg.ErrorCode,
				Message: *msg.Error,
			}
		}
		return msg.Result, nil
	case <-time.After(timeout):
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("timeout after %s", timeout)
	}
}

// CallWithResult sends a request and decodes the response into resultPtr.
func (c *Connection) CallWithResult(method string, params map[string]any, timeout time.Duration, resultPtr any) error {
	res, err := c.Call(method, params, timeout)
	if err != nil {
		return err
	}
	return decodeInto(resultPtr, res)
}

// CallAsync sends a request and calls the callback when the response arrives.
func (c *Connection) CallAsync(method string, params map[string]any, timeout time.Duration, callback func(any, error)) *CallContext {
	id := uuid.NewString()
	ch := make(chan RPCMessage, 1)

	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	req := RPCMessage{
		Type:   RequestType,
		ID:     id,
		Method: method,
		Params: params,
	}

	if err := c.Send(req); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		if callback != nil {
			go callback(nil, err)
		}
		return nil
	}

	ctx := &CallContext{
		ID:       id,
		cancelCh: make(chan struct{}),
		conn:     c,
	}

	go func() {
		defer func() {
			c.pendingMu.Lock()
			delete(c.pending, id)
			c.pendingMu.Unlock()
		}()

		select {
		case msg := <-ch:
			if callback != nil {
				if msg.Error != nil {
					callback(nil, fmt.Errorf("error code %d: %s", msg.ErrorCode, *msg.Error))
				} else {
					callback(msg.Result, nil)
				}
			}
		case <-time.After(timeout):
			if callback != nil {
				callback(nil, fmt.Errorf("timeout after %s", timeout))
			}
		case <-ctx.cancelCh:
			if callback != nil {
				callback(nil, fmt.Errorf("call cancelled"))
			}
		}
	}()

	return ctx
}

// CallAsyncWithResult sends a request and decodes result into resultPtr.
func (c *Connection) CallAsyncWithResult(method string, params map[string]any, timeout time.Duration, resultPtr any, callback func(error)) *CallContext {
	return c.CallAsync(method, params, timeout, func(res any, err error) {
		if err != nil {
			if callback != nil {
				callback(err)
			}
			return
		}
		if callback != nil {
			callback(decodeInto(resultPtr, res))
		}
	})
}

type CallContext struct {
	ID       string
	cancelCh chan struct{}
	conn     *Connection
}

func (cc *CallContext) Cancel() {
	select {
	case <-cc.cancelCh:
	default:
		close(cc.cancelCh)
	}
}

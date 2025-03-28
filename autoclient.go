package bidirpc

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type AutoClient struct {
	addr           string
	clientID       string
	authCode       string
	useTLS         bool
	tlsConfig      *tls.Config
	ALPN           string
	useCompression bool
	onReady        func(*Connection)
	stopChan       chan struct{}
	wg             sync.WaitGroup
	mu             sync.Mutex
	stopped        bool
	handlers       *HandlerRegistry
	activeConn     atomic.Value // stores *Connection
}

// NewAutoClient creates an AutoClient instance ready to connect.
func NewAutoClient(addr, clientID, authCode string, useTLS bool, tlsConfig *tls.Config, ALPN string, useCompression bool, onReady func(*Connection)) *AutoClient {
	return &AutoClient{
		addr:           addr,
		clientID:       clientID,
		authCode:       authCode,
		useTLS:         useTLS,
		tlsConfig:      tlsConfig,
		ALPN:           ALPN,
		useCompression: useCompression,
		onReady:        onReady,
		stopChan:       make(chan struct{}),
		handlers:       NewHandlerRegistry(),
	}
}

// RegisterHandler registers a handler before starting the client.
func (ac *AutoClient) RegisterHandler(method string, fn HandlerFunc) {
	ac.handlers.Register(method, fn)
}

// Start initiates the first connection and begins auto-reconnect loop.
// Returns error if the first connection attempt fails.
func (ac *AutoClient) Start() error {
	ready := make(chan error, 1)

	go func() {
		err := ac.connectOnce()
		ready <- err
		if err == nil {
			ac.wg.Add(1)
			go func() {
				defer ac.wg.Done()
				ac.loop()
			}()
		}
	}()

	select {
	case err := <-ready:
		return err
	case <-time.After(5 * time.Second):
		return fmt.Errorf("initial connection timeout")
	}
}

func (ac *AutoClient) loop() {
	attempt := 1
	for {
		select {
		case <-ac.stopChan:
			return
		default:
			err := ac.connectOnce()
			if err != nil {
				log.Printf("[client] reconnect failed (attempt %d): %v", attempt, err)
				delay := backoffDuration(attempt)
				time.Sleep(delay)
				attempt++
			} else {
				attempt = 1 // Reset on successful connection
			}
		}
	}
}

func backoffDuration(attempt int) time.Duration {
	base := time.Second * 2
	delay := base << (attempt - 1) // 2s, 4s, 8s, ...
	if delay > 3*time.Minute {
		return 3 * time.Minute
	}
	return delay
}

func (ac *AutoClient) connectOnce() error {
	var conn net.Conn
	var err error

	if ac.useTLS {
		tlsConf := ac.tlsConfig.Clone()
		tlsConf.NextProtos = []string{ac.ALPN}
		conn, err = tls.Dial("tcp", ac.addr, tlsConf)
	} else {
		conn, err = net.Dial("tcp", ac.addr)
	}
	if err != nil {
		log.Println("[client] connection error:", err)
		return err
	}

	c := NewConnection(conn)

	// Send negotiation
	err = c.SendNegotiation(NegotiationMessage{
		Type:           AuthRequestType,
		ClientID:       ac.clientID,
		AuthCode:       ac.authCode,
		UseCompression: ac.useCompression,
	})
	if err != nil {
		log.Println("[client] failed to send negotiation:", err)
		conn.Close()
		return err
	}

	var resp NegotiationMessage
	err = c.ReceiveNegotiation(&resp)
	if err != nil {
		log.Println("[client] failed to receive negotiation:", err)
		conn.Close()
		return err
	}

	if resp.Type != AuthOKType {
		log.Println("[client] server rejected authentication")
		conn.Close()
		return fmt.Errorf("authentication failed")
	}

	if resp.UseCompression {
		if err := c.EnableCompression(); err != nil {
			log.Println("[client] compression failed:", err)
			conn.Close()
			return err
		}
	}

	// Initialize handlers and reader
	c.handlers = ac.handlers
	c.StartReadLoop()
	ac.activeConn.Store(c)

	if ac.onReady != nil {
		ac.onReady(c)
	}

	return nil
}

// Call performs a blocking RPC call using the active connection.
func (ac *AutoClient) Call(method string, params map[string]any, timeout time.Duration) (any, error) {
	value := ac.activeConn.Load()
	if value == nil {
		return nil, fmt.Errorf("client is not connected")
	}
	return value.(*Connection).Call(method, params, timeout)
}

// CallWithResult performs a blocking RPC call and decodes into resultPtr.
func (ac *AutoClient) CallWithResult(method string, params map[string]any, timeout time.Duration, resultPtr any) error {
	value := ac.activeConn.Load()
	if value == nil {
		return fmt.Errorf("client is not connected")
	}
	return value.(*Connection).CallWithResult(method, params, timeout, resultPtr)
}

// CallAsync performs an async RPC call with a callback.
func (ac *AutoClient) CallAsync(method string, params map[string]any, timeout time.Duration, callback func(any, error)) error {
	value := ac.activeConn.Load()
	if value == nil {
		return fmt.Errorf("client is not connected")
	}
	value.(*Connection).CallAsync(method, params, timeout, callback)
	return nil
}

// CallAsyncWithResult performs an async RPC call and decodes into resultPtr.
func (ac *AutoClient) CallAsyncWithResult(method string, params map[string]any, timeout time.Duration, resultPtr any, callback func(error)) error {
	value := ac.activeConn.Load()
	if value == nil {
		return fmt.Errorf("client is not connected")
	}
	value.(*Connection).CallAsyncWithResult(method, params, timeout, resultPtr, callback)
	return nil
}

// IsConnected returns true if a connection is active.
func (ac *AutoClient) IsConnected() bool {
	return ac.activeConn.Load() != nil
}

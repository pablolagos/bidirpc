package bidirpc

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

type Server struct {
	authFunc  func(clientID, authCode string) bool
	handlers  *HandlerRegistry
	clients   map[string]*Connection
	clientsMu sync.RWMutex
}

// NewServer creates a new RPC server with address and authentication function.
func NewServer(authFunc func(clientID, authCode string) bool) *Server {
	return &Server{
		authFunc: authFunc,
		handlers: NewHandlerRegistry(),
		clients:  make(map[string]*Connection),
	}
}

// RegisterHandler registers an RPC handler.
func (s *Server) RegisterHandler(method string, fn HandlerFunc) {
	s.handlers.Register(method, fn)
}

// ServeConn handles an incoming client connection.
func (s *Server) ServeConn(conn net.Conn) {
	c := NewConnection(conn)

	// Read negotiation message
	var negMsg NegotiationMessage
	if err := c.ReceiveNegotiation(&negMsg); err != nil {
		log.Println("[server] failed to receive negotiation:", err)
		conn.Close()
		return
	}

	if negMsg.Type != AuthRequestType {
		log.Println("[server] unexpected negotiation type")
		conn.Close()
		return
	}

	if !s.authFunc(negMsg.ClientID, negMsg.AuthCode) {
		log.Println("[server] authentication failed for client:", negMsg.ClientID)
		resp := NegotiationMessage{Type: AuthFailType}
		_ = c.SendNegotiation(resp)
		conn.Close()
		return
	}

	// Send AuthOK (without compression yet)
	resp := NegotiationMessage{
		Type:           AuthOKType,
		UseCompression: negMsg.UseCompression,
	}
	if err := c.SendNegotiation(resp); err != nil {
		log.Println("[server] failed to send AuthOK:", err)
		conn.Close()
		return
	}

	// Enable compression if agreed
	if negMsg.UseCompression {
		if err := c.EnableCompression(); err != nil {
			log.Println("[server] failed to enable compression:", err)
			conn.Close()
			return
		}
	}

	c.handlers = s.handlers

	s.clientsMu.Lock()
	s.clients[negMsg.ClientID] = c
	s.clientsMu.Unlock()

	log.Println("[server] client connected:", negMsg.ClientID)

	c.StartReadLoop()

	// When connection dies, cleanup
	go func() {
		for {
			time.Sleep(1 * time.Second)
			if c.Dec == nil {
				break
			}
		}
		log.Println("[server] disconnected:", negMsg.ClientID)
		s.clientsMu.Lock()
		delete(s.clients, negMsg.ClientID)
		s.clientsMu.Unlock()
	}()
}

// GetClientByID returns the active connection for a given client.
func (s *Server) GetClientByID(clientID string) *Connection {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	return s.clients[clientID]
}

// Call sends a blocking RPC call to a client.
func (s *Server) Call(clientID, method string, params map[string]any, timeout time.Duration) (any, error) {
	conn := s.GetClientByID(clientID)
	if conn == nil {
		return nil, fmt.Errorf("client %s is not connected", clientID)
	}
	return conn.Call(method, params, timeout)
}

// CallWithResult sends a blocking RPC call and decodes the result into resultPtr.
func (s *Server) CallWithResult(clientID, method string, params map[string]any, timeout time.Duration, resultPtr any) error {
	conn := s.GetClientByID(clientID)
	if conn == nil {
		return fmt.Errorf("client %s is not connected", clientID)
	}
	return conn.CallWithResult(method, params, timeout, resultPtr)
}

// CallAsync sends an async call with callback.
func (s *Server) CallAsync(clientID, method string, params map[string]any, timeout time.Duration, callback func(any, error)) error {
	conn := s.GetClientByID(clientID)
	if conn == nil {
		return fmt.Errorf("client %s is not connected", clientID)
	}
	conn.CallAsync(method, params, timeout, callback)
	return nil
}

// CallAsyncWithResult sends an async call and decodes result into resultPtr.
func (s *Server) CallAsyncWithResult(clientID, method string, params map[string]any, timeout time.Duration, resultPtr any, callback func(error)) error {
	conn := s.GetClientByID(clientID)
	if conn == nil {
		return fmt.Errorf("client %s is not connected", clientID)
	}
	conn.CallAsyncWithResult(method, params, timeout, resultPtr, callback)
	return nil
}

// Serve starts a plain TCP server and accepts incoming connections.
func (s *Server) Serve(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	log.Println("[server] listening on", addr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("[server] accept error:", err)
			continue
		}
		go s.ServeConn(conn)
	}
}

// ServeTLS starts a TLS server and accepts incoming connections.
func (s *Server) ServeTLS(addr string, tlsConfig *tls.Config) error {
	ln, err := tls.Listen("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to listen TLS on %s: %w", addr, err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Println("[server] TLS accept error:", err)
				continue
			}
			go s.ServeConn(conn)
		}
	}()

	return nil
}

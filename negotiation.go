package bidirpc

import (
	"encoding/json"
)

// NegotiationMessage is used for the initial handshake between client and server.
type NegotiationMessage struct {
	Type           MessageType `json:"type"`                     // Message type: auth_request, auth_ok, etc.
	ClientID       string      `json:"clientID,omitempty"`       // Sent by client
	AuthCode       string      `json:"authCode,omitempty"`       // Sent by client
	UseCompression bool        `json:"useCompression,omitempty"` // Request or confirm gzip compression
}

// Negotiation message types
const (
	AuthRequestType = "auth_request"
	AuthFailType    = "auth_fail"
)

// SendNegotiation sends a negotiation message directly (without compression).
func (c *Connection) SendNegotiation(msg NegotiationMessage) error {
	return json.NewEncoder(c.Conn).Encode(msg)
}

// ReceiveNegotiation reads a negotiation message directly from the raw connection.
func (c *Connection) ReceiveNegotiation(msg *NegotiationMessage) error {
	return json.NewDecoder(c.Conn).Decode(msg)
}

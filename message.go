package bidirpc

// MessageType represents the type of a message.
type MessageType string

const (
	AuthType     MessageType = "auth"
	AuthOKType   MessageType = "auth_ok"
	AuthErrType  MessageType = "auth_error"
	RequestType  MessageType = "request"
	ResponseType MessageType = "response"
)

// RPCMessage is used for the exchange of RPC requests and responses.
type RPCMessage struct {
	Type      MessageType    `json:"type"`
	ID        string         `json:"id,omitempty"`
	Method    string         `json:"method,omitempty"`
	Params    map[string]any `json:"params,omitempty"`
	Result    any            `json:"result,omitempty"`
	Error     *string        `json:"error,omitempty"`
	ErrorCode int            `json:"errorCode,omitempty"`
}

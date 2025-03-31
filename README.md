# bidirpc

**Reliable two-way RPC for modern Go applications.**

**bidirpc** lets you build real-time, two-way communication between servers and clients over persistent TCP connections—securely and efficiently. With built-in support for TLS, optional gzip compression, and seamless reconnection, both sides can register and call remote functions as if they were local.

Whether you're building dashboards, remote control systems, messaging backends, or IoT communication, `bidirpc` gives you a clean and powerful foundation.

---

## ✨ Key Features

- Minimal and expressive API
- Server and client can both register and call functions
- Automatic reconnection with exponential backoff
- TLS encryption and ALPN support
- Optional gzip compression
- Built-in context helpers for handlers
- Multi-client server support with clientID routing
- Explicit context structure for robust handler design
- Keep-alive system with heartbeat and timeout detection

---

## 📦 Installation

```bash
go get github.com/pablolagos/bidirpc
```

---

## 🚀 How It Works

`bidirpc` is built around long-lived TCP connections. When a client connects:

1. It sends its `clientID`, `authCode`, and compression preference.
2. The server authenticates the client.
3. If both sides support compression, it is enabled.
4. From that point, both client and server can call each other’s functions.

Connections remain open, allowing low-latency communication.

Clients also send periodic heartbeat pings. If the server doesn't receive a ping in 40 seconds, it marks the client as disconnected.

---

## 🖥️ Quick Example

### Server
```go
server := bidirpc.NewServer(func(id, code string) bool {
    return code == "s3cr3t"
})

server.RegisterHandler("Greet", func(ctx *bidirpc.Context) {
    name := ctx.GetParamString("name", "there")
    ctx.WriteResponse("Hello, " + name + "!")
})

log.Fatal(server.ServeTLS(":8443", tlsConfig))
```

### Client
```go
client := bidirpc.NewAutoClient(
    "127.0.0.1:8443", "client42", "s3cr3t", true, tlsConfig, "bidirpc", true,
    func(conn *bidirpc.Connection) {
        log.Println("[connected]")
    },
)

if err := client.Start(); err != nil {
    log.Fatal(err)
}

// Once connected, call the server
var res string
client.CallWithResult("Greet", map[string]any{"name": "Pablo"}, 3*time.Second, &res)
log.Println("Server says:", res)
```

---

## 🔁 Call Methods

You can call remote functions in different ways:

### Synchronous:
```go
var result string
err := conn.CallWithResult("Hello", nil, 3*time.Second, &result)
```

### Asynchronous:
```go
conn.CallAsync("Hello", nil, 3*time.Second, func(res any, err error) {
    if err != nil {
        log.Println("Error:", err)
    } else {
        log.Println("Result:", res)
    }
})
```

---

## 🧠 Writing Handlers

Handlers receive a `*Context` object with helpers:
```go
ctx.GetParamString("key", "default")
ctx.GetParamInt("key", 0)
ctx.WriteResponse(data)
ctx.WriteError(400, "error")
ctx.ClientID() // get clientID of the requester
```

The context contains the request parameters and request ID explicitly, making the design clear and bug-resistant.

---

## 🔐 Security & ALPN

- TLS support via `tls.Config`
- ALPN lets you negotiate the protocol (e.g., "bidirpc")
- Client sends `clientID` and `authCode`, validated by your function
- Gzip compression is negotiated automatically

---

## 🔄 Auto-Reconnect & Keep-Alive

Clients reconnect automatically with exponential backoff (up to 3 minutes).

Clients also send `Ping` requests every 30 seconds. The server stores the last time each client pinged. If no ping is received within 40 seconds, the client is considered inactive and removed from the active connection list.

---

## 🔑 Authentication and Client Management

When a client connects, it must provide a `clientID` and an `authCode`. The server uses a user-defined function to validate this information. If the authentication fails, the connection is rejected.

All authenticated clients are stored in memory on the server and mapped by their unique `clientID`. This enables targeted messaging, monitoring, and disconnection control.

### 📌 Call a specific client:
```go
conn := server.GetClientByID("client42")
if conn != nil {
    var response string
    conn.CallWithResult("Ping", nil, 2*time.Second, &response)
}
```

### 📣 Broadcast to all clients:
```go
for _, id := range server.ListClientIDs() {
    if c := server.GetClientByID(id); c != nil {
        c.CallAsync("Notify", map[string]any{"msg": "Hello!"}, 2*time.Second, nil)
    }
}
```

---

## 📜 License

MIT © Pablo Lagos


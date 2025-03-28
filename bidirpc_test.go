package bidirpc_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"log"
	"math/big"
	"testing"
	"time"

	"github.com/pablolagos/bidirpc"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type BidiRPCServerSuite struct {
	suite.Suite
	server   *bidirpc.Server
	client   *bidirpc.AutoClient
	clientID string
	authCode string
}

// Make sure that VariableThatShouldStartAtFive is set to five
// before each test
func (s *BidiRPCServerSuite) SetupSuite() {
	s.clientID = "client42"
	s.authCode = "s3cr3t"
	s.server = s.startServer()
	s.client = s.startClient()
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestBidiRPCServerSuite(t *testing.T) {
	suite.Run(t, new(BidiRPCServerSuite))
}

func (s *BidiRPCServerSuite) startServer() *bidirpc.Server {
	server := bidirpc.NewServer(func(clientID, authCode string) bool {
		return clientID == "client42" && authCode == "s3cr3t"
	})

	server.RegisterHandler("Echo", func(ctx *bidirpc.Context) {
		msg := ctx.GetParamString("msg", "")
		ctx.WriteResponse(msg)
	})

	server.RegisterHandler("Divide", func(ctx *bidirpc.Context) {
		num := ctx.GetParamInt("num", 0)
		denom := ctx.GetParamInt("denom", 1)
		if denom == 0 {
			ctx.WriteError(44, "division by zero")
			return
		}
		result := num / denom
		ctx.WriteResponse(result)
	})

	cert := generateSelfSignedCert(s.T())
	err := server.ServeTLS("127.0.0.1:9443", &tls.Config{
		NextProtos:   []string{"bidirpc"},
		Certificates: []tls.Certificate{cert},
	})
	require.NoError(s.T(), err, "server.ServeTLS")

	return server
}

func (s *BidiRPCServerSuite) startClient() *bidirpc.AutoClient {
	client := bidirpc.NewAutoClient("127.0.0.1:9443", s.clientID, s.authCode, true, &tls.Config{InsecureSkipVerify: true}, "bidirpc", true, nil)
	client.RegisterHandler("Multiply", func(ctx *bidirpc.Context) {
		num := ctx.GetParamInt("num", 0)
		factor := ctx.GetParamInt("factor", 1)
		result := num * factor
		ctx.WriteResponse(result)
	})

	s.T().Log("[client] starting client")
	err := client.Start()
	require.NoError(s.T(), err, "client.Start")

	return client
}

// Test calling echo method form client
func (s *BidiRPCServerSuite) TestEchoClientServer() {
	var reply string
	err := s.client.CallWithResult("Echo", map[string]any{"msg": "ping"}, 300*time.Second, &reply)
	require.NoError(s.T(), err, "CallWithResult")
	require.Equal(s.T(), "ping", reply, "unexpected reply")
}

// Test calling divide method form client with a normal result
func (s *BidiRPCServerSuite) TestDivideClientServer() {
	var reply int
	err := s.client.CallWithResult("Divide", map[string]any{"num": 10, "denom": 2}, 300*time.Second, &reply)
	require.NoError(s.T(), err, "CallWithResult")
	require.Equal(s.T(), 5, reply, "unexpected reply")
}

// Test calling divide method form client with a division by zero error
func (s *BidiRPCServerSuite) TestDivideClientServerDivisionByZero() {
	var reply int
	err := s.client.CallWithResult("Divide", map[string]any{"num": 10, "denom": 0}, 300*time.Second, &reply)
	var expectedErr *bidirpc.ResponseError
	if !errors.As(err, &expectedErr) {
		s.T().Fatal("expected ResponseError")
	}
	require.Equal(s.T(), 44, expectedErr.Code, "unexpected error code")
	require.Equal(s.T(), "division by zero", expectedErr.Message, "unexpected error message")
}

// Test calling multiply method form server
func (s *BidiRPCServerSuite) TestMultiplyServerClient() {
	var reply int
	err := s.server.CallWithResult(s.clientID, "Multiply", map[string]any{"num": 10, "factor": 2}, 300*time.Second, &reply)
	require.NoError(s.T(), err, "CallWithResult")
	require.Equal(s.T(), 20, reply, "unexpected reply")
}

func Test_RealServerAndClient_Integration(t *testing.T) {
	addr := "127.0.0.1:9443"
	clientID := "client42"
	authCode := "s3cr3t"

	// Generate self-signed TLS certificate
	cert := generateSelfSignedCert(t)
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAnyClientCert,
		NextProtos:   []string{"bidirpc"},
	}

	// Start server
	server := bidirpc.NewServer(func(id, code string) bool {
		return id == clientID && code == authCode
	})

	ln, err := tls.Listen("tcp", addr, tlsConfig)
	if err != nil {
		t.Fatal("server listen failed:", err)
	}
	defer ln.Close()

	go func() {
		log.Println("[server] listening on", addr)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go server.ServeConn(conn)
		}
	}()

	time.Sleep(300 * time.Millisecond) // allow server to start

	// Create AutoClient
	clientTLS := tlsConfig.Clone()
	clientTLS.InsecureSkipVerify = true

	var connectedConn *bidirpc.Connection
	client := bidirpc.NewAutoClient(
		addr,
		clientID,
		authCode,
		true,
		clientTLS,
		"bidirpc",
		true,
		func(conn *bidirpc.Connection) {
			connectedConn = conn
		},
	)

	// Register handler before connecting
	client.RegisterHandler("HelloFromServer", func(ctx *bidirpc.Context) {
		ctx.WriteResponse("hello received")
	})

	// Start and ensure first connection succeeds
	if err := client.Start(); err != nil {
		t.Fatal("client failed to connect:", err)
	}

	// Wait for onReady to run
	var retries int
	for retries = 0; retries < 50 && connectedConn == nil; retries++ {
		time.Sleep(100 * time.Millisecond)
	}
	if connectedConn == nil {
		t.Fatal("onReady was not called")
	}

	// Make a call to the server
	var reply string
	err = connectedConn.CallWithResult("Echo", map[string]any{"msg": "ping"}, 300*time.Second, &reply)
	if err != nil {
		t.Fatal("CallWithResult failed:", err)
	}
	if reply != "ping" {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func generateSelfSignedCert(t *testing.T) tls.Certificate {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}
	der, _ := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal("X509KeyPair failed:", err)
	}
	return cert
}

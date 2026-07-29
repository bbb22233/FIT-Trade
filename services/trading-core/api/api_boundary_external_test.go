package api_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"net"
	"reflect"
	"testing"

	"fit.trade/trading-core/api"
	"fit.trade/trading-core/auth"
)

type loopbackAddrSpoof struct{ net.Listener }

func (loopbackAddrSpoof) Addr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1}
}

func TestExternalServerMethodSetHasNoRawHTTPIngress(t *testing.T) {
	server, err := api.NewLoopbackServer(auth.NewService(auth.Config{}), bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	methods := reflect.TypeOf(server)
	for _, name := range []string{"Handler", "ServeHTTP", "HTTPHandler"} {
		if method, found := methods.MethodByName(name); found {
			t.Fatalf("external Server method %s exposes raw HTTP ingress: %s", name, method.Type)
		}
	}
}

func TestExternalServeRejectsPublicAndAddrSpoofListeners(t *testing.T) {
	server, err := api.NewLoopbackServer(auth.NewService(auth.Config{}), bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Listen(context.Background(), "0.0.0.0:0"); err == nil {
		t.Fatal("public listener bind accepted")
	}
	public, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer public.Close()
	if err := server.Serve(public); err == nil {
		t.Fatal("public listener accepted by Serve")
	}
	spoof := loopbackAddrSpoof{Listener: public}
	if err := server.Serve(spoof); err == nil {
		t.Fatal("Addr-spoofed listener accepted by Serve")
	}
	if err := server.ServeTLS(spoof, &tls.Config{Certificates: []tls.Certificate{{}}}); err == nil {
		t.Fatal("Addr-spoofed listener accepted by ServeTLS")
	}
}

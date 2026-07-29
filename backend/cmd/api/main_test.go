package main

import (
	"net/http"
	"testing"

	httpdelivery "github.com/royle/construction/backend/internal/delivery/http"
)

func TestNewHTTPServerHasBoundedProductionTimeouts(t *testing.T) {
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	server := newHTTPServer("18080", handler)
	if server.Addr != ":18080" || server.Handler == nil {
		t.Fatalf("server address/handler = %q %#v", server.Addr, server.Handler)
	}
	if server.ReadHeaderTimeout != httpReadHeaderTimeout || server.ReadTimeout != httpReadTimeout ||
		server.WriteTimeout != httpWriteTimeout || server.IdleTimeout != httpIdleTimeout {
		t.Fatalf("unexpected timeouts: header=%s read=%s write=%s idle=%s",
			server.ReadHeaderTimeout, server.ReadTimeout, server.WriteTimeout, server.IdleTimeout)
	}
	if server.ReadHeaderTimeout <= 0 || server.ReadTimeout <= 0 || server.IdleTimeout <= 0 {
		t.Fatal("all connection lifecycle timeouts must be bounded")
	}
	if server.WriteTimeout <= httpdelivery.DocumentPDFWorkTimeout {
		t.Fatalf("write timeout %s does not leave delivery margin after PDF work", server.WriteTimeout)
	}
}

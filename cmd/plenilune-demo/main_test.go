package main

import (
	"net/http"
	"testing"
	"time"
)

func TestNewServerAppliesHardeningTimeouts(t *testing.T) {
	server := newServer("127.0.0.1:8080", http.NotFoundHandler())
	if server.Addr != "127.0.0.1:8080" {
		t.Fatalf("addr = %q", server.Addr)
	}
	for _, tc := range []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{"ReadHeaderTimeout", server.ReadHeaderTimeout, 5 * time.Second},
		{"ReadTimeout", server.ReadTimeout, 60 * time.Second},
		{"WriteTimeout", server.WriteTimeout, 180 * time.Second},
		{"IdleTimeout", server.IdleTimeout, 60 * time.Second},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
	if server.MaxHeaderBytes != 1<<20 {
		t.Errorf("MaxHeaderBytes = %d, want %d", server.MaxHeaderBytes, 1<<20)
	}
}

func TestLoopbackAddress(t *testing.T) {
	for _, tc := range []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:8080", true},
		{"127.0.0.2:9000", true},
		{"[::1]:8080", true},
		{"localhost:8080", true},
		{"0.0.0.0:8080", false},
		{"[::]:8080", false},
		{":8080", false},
		{"192.168.1.10:8080", false},
		{"example.com:8080", false},
		{"127.0.0.1", false},
		{"", false},
	} {
		if got := loopbackAddress(tc.addr); got != tc.want {
			t.Errorf("loopbackAddress(%q) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}

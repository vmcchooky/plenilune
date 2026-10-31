package main

import "testing"

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

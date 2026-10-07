package common

import "testing"

func TestIsIPv4Literal(t *testing.T) {
	ok := []string{"192.168.1.1", " 192.168.1.1 ", "0.0.0.0", "255.255.255.255"}
	for _, s := range ok {
		if !IsIPv4Literal(s) {
			t.Errorf("IsIPv4Literal(%q) 应为 true", s)
		}
	}
	bad := []string{"256.1.1.1", "::1", "192.168.1", "192.168.1.1:22", "host.example.com", "", "1.2.3.4/24"}
	for _, s := range bad {
		if IsIPv4Literal(s) {
			t.Errorf("IsIPv4Literal(%q) 应为 false", s)
		}
	}
}

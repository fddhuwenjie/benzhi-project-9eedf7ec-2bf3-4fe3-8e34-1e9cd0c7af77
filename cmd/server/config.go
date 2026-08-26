package main

import (
	"net"
	"strings"
)

func validLoopback(addr string) bool {
	h, _, e := net.SplitHostPort(addr)
	if e != nil {
		return false
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback() && strings.TrimSpace(addr) != ""
}

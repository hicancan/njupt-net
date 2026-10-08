//go:build windows || linux || darwin

package network

import (
	"context"
	"errors"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSourceInterfaceRequiresUniqueActiveAssignment(t *testing.T) {
	for _, test := range []struct {
		name  string
		items []Interface
		index int
	}{
		{"assigned", []Interface{{Index: 7, Up: true, IPv4: []string{"192.0.2.1"}}}, 7},
		{"down duplicate", []Interface{{Index: 7, Up: true, IPv4: []string{"192.0.2.1"}}, {Index: 8, IPv4: []string{"192.0.2.1"}}}, 7},
		{"same interface", []Interface{{Index: 7, Up: true, IPv4: []string{"192.0.2.1", "192.0.2.1"}}}, 7},
		{"missing", []Interface{{Index: 7, Up: true, IPv4: []string{"192.0.2.2"}}}, 0},
		{"down", []Interface{{Index: 7, IPv4: []string{"192.0.2.1"}}}, 0},
		{"ambiguous", []Interface{{Index: 7, Up: true, IPv4: []string{"192.0.2.1"}}, {Index: 8, Up: true, IPv4: []string{"192.0.2.1"}}}, 0},
		{"invalid index", []Interface{{Up: true, IPv4: []string{"192.0.2.1"}}}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			index, err := sourceInterface(test.items, "192.0.2.1")
			if index != test.index || (err != nil) != (test.index == 0) {
				t.Fatalf("index=%d error=%v; want index %d", index, err, test.index)
			}
		})
	}
}

func TestLinkBindsConnectedTCPSocketToSourceInterface(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	link, err := NewLink("127.0.0.1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer link.Close()
	items, err := Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	expectedIndex, err := sourceInterface(items, link.Source())
	if err != nil {
		t.Fatal(err)
	}
	connection, err := link.transport.DialContext(context.Background(), "tcp4", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	raw, err := connection.(*net.TCPConn).SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var index int
	var optionErr error
	if err := raw.Control(func(fd uintptr) { index, optionErr = getSocketInterface(fd) }); err != nil {
		t.Fatal(err)
	}
	if optionErr != nil || index != expectedIndex {
		t.Fatalf("connected TCP interface=%d error=%v; want %d", index, optionErr, expectedIndex)
	}
}

type failedRawConn struct{ controlErr error }

func (c failedRawConn) Control(callback func(uintptr)) error {
	if c.controlErr != nil {
		return c.controlErr
	}
	callback(^uintptr(0))
	return nil
}
func (failedRawConn) Read(func(uintptr) bool) error  { panic("unused") }
func (failedRawConn) Write(func(uintptr) bool) error { panic("unused") }

func TestInterfaceControlReturnsNativeFailures(t *testing.T) {
	control := interfaceControl(1)
	controlErr := errors.New("socket closed")
	if err := control("tcp4", "127.0.0.1:1", failedRawConn{controlErr}); !errors.Is(err, controlErr) {
		t.Fatalf("socket control failure was lost: %v", err)
	}
	err := control("tcp4", "127.0.0.1:1", failedRawConn{})
	var nativeErr syscall.Errno
	if !errors.As(err, &nativeErr) || !strings.Contains(err.Error(), "bind IPv4 socket to interface 1") {
		t.Fatalf("native binding failure was ignored or lost: %v", err)
	}
}

package network

import (
	"errors"
	"reflect"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWindowsAdapterSnapshotPreservesInterfaceIdentityAndIPv4(t *testing.T) {
	name, err := windows.UTF16PtrFromString("校园网络")
	if err != nil {
		t.Fatal(err)
	}
	var ipv4, ipv6, secondIPv4 syscall.RawSockaddrAny
	*(*windows.RawSockaddrInet4)(unsafe.Pointer(&ipv4)) = windows.RawSockaddrInet4{Family: windows.AF_INET, Addr: [4]byte{10, 20, 30, 40}}
	*(*windows.RawSockaddrInet6)(unsafe.Pointer(&ipv6)) = windows.RawSockaddrInet6{Family: windows.AF_INET6, Addr: [16]byte{0x20, 0x01, 0x0d, 0xb8}}
	*(*windows.RawSockaddrInet4)(unsafe.Pointer(&secondIPv4)) = windows.RawSockaddrInet4{Family: windows.AF_INET, Addr: [4]byte{10, 20, 30, 41}}
	addresses := []windows.IpAdapterUnicastAddress{
		{Address: windows.SocketAddress{Sockaddr: &ipv4, SockaddrLength: int32(unsafe.Sizeof(windows.RawSockaddrInet4{}))}},
		{Address: windows.SocketAddress{Sockaddr: &ipv6, SockaddrLength: int32(unsafe.Sizeof(windows.RawSockaddrInet6{}))}},
		{Address: windows.SocketAddress{Sockaddr: &secondIPv4, SockaddrLength: int32(unsafe.Sizeof(windows.RawSockaddrInet4{}))}},
	}
	addresses[0].Next = &addresses[1]
	addresses[1].Next = &addresses[2]
	adapters := []windows.IpAdapterAddresses{
		{FriendlyName: name, IfIndex: 12, OperStatus: windows.IfOperStatusUp, FirstUnicastAddress: &addresses[0]},
		{Ipv6IfIndex: 23, OperStatus: windows.IfOperStatusDown},
	}
	adapters[0].Next = &adapters[1]
	want := []Interface{
		{Name: "校园网络", Index: 12, Up: true, IPv4: []string{"10.20.30.40", "10.20.30.41"}},
		{Index: 23, Up: false, IPv4: []string{}},
	}
	if got, err := adapterInterfaces(&adapters[0], nil); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("adapter snapshot=%+v, error=%v, want %+v", got, err, want)
	}
	if got, err := adapterInterfaces(nil, nil); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty snapshot=%v, error=%v", got, err)
	}
}

func TestWindowsAdapterSnapshotDistinguishesNoDataFromFailure(t *testing.T) {
	stale := &windows.IpAdapterAddresses{IfIndex: 12, OperStatus: windows.IfOperStatusUp}
	if got, err := adapterInterfaces(stale, windows.ERROR_NO_DATA); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("native no-data result=%v, error=%v", got, err)
	}
	for _, native := range []error{windows.ERROR_ACCESS_DENIED, windows.ERROR_BUFFER_OVERFLOW} {
		got, err := adapterInterfaces(stale, native)
		if got != nil || !errors.Is(err, native) {
			t.Fatalf("native failure=%v, result=%v, error=%v", native, got, err)
		}
	}
}

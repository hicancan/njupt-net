package network

import (
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Interfaces reads interface identities and unicast addresses in one snapshot.
// The initial buffer follows GetAdaptersAddresses' recommended working size.
func Interfaces() ([]Interface, error) {
	length := uint32(15000)
	for {
		buffer := make([]byte, length)
		first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buffer[0]))
		const flags = windows.GAA_FLAG_SKIP_ANYCAST | windows.GAA_FLAG_SKIP_MULTICAST | windows.GAA_FLAG_SKIP_DNS_SERVER
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, flags, 0, first, &length)
		if err == windows.ERROR_BUFFER_OVERFLOW && length > uint32(len(buffer)) {
			continue
		}
		if length == 0 {
			first = nil
		}
		result, err := adapterInterfaces(first, err)
		runtime.KeepAlive(buffer)
		return result, err
	}
}

func adapterInterfaces(first *windows.IpAdapterAddresses, err error) ([]Interface, error) {
	if err == windows.ERROR_NO_DATA {
		return []Interface{}, nil
	}
	if err != nil {
		return nil, os.NewSyscallError("GetAdaptersAddresses", err)
	}
	result := []Interface{}
	for adapter := first; adapter != nil; adapter = adapter.Next {
		index := adapter.IfIndex
		if index == 0 {
			index = adapter.Ipv6IfIndex
		}
		entry := Interface{
			Name: windows.UTF16PtrToString(adapter.FriendlyName), Index: int(index),
			Up: adapter.OperStatus == windows.IfOperStatusUp, IPv4: []string{},
		}
		for address := adapter.FirstUnicastAddress; address != nil; address = address.Next {
			if ip := address.Address.IP().To4(); ip != nil {
				entry.IPv4 = append(entry.IPv4, ip.String())
			}
		}
		result = append(result, entry)
	}
	return result, nil
}

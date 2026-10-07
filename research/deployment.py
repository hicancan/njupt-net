"""Inspect HTTP entrances, one selected UDP protocol, or an Nmap XML observation."""

import argparse
from datetime import datetime, timezone
import ipaddress
import json
from pathlib import Path
import socket
import xml.etree.ElementTree as ET
from urllib.parse import urlsplit
from network import Link
from pages import Page

ENDPOINTS = (
    "https://p.njupt.edu.cn/a79.htm",
    "http://p.njupt.edu.cn:801/eportal/portal/index",
    "https://p.njupt.edu.cn:802/eportal/portal/index",
    "http://p.njupt.edu.cn:803/eportal/portal/index",
    "https://p.njupt.edu.cn:804/eportal/portal/index",
    "http://zfw.njupt.edu.cn:8080/Self/login",
)

UDP_PROBES = {
    "ntp": (123, b"\x1b" + bytes(47)),
    "drcom-challenge": (61440, bytes.fromhex("07 01 08 00 01 00 00 00")),
}


def udp_evidence(probe, data):
    if probe == "ntp":
        header = data[:4]
        fields = {"version": (data[0] >> 3) & 7, "mode": data[0] & 7,
                  "leap": data[0] >> 6, "stratum": data[1]} if len(data) >= 2 else {}
        matches = len(data) >= 48 and fields["version"] in {3, 4} and fields["mode"] == 4
    else:
        # The reference client reads the seed at [8:12] and address at [12:16].
        # Publish the preceding header and structural fields only.
        header = data[:8]
        fields = {"code": data[0], "request_id": data[1],
                  "length_field": int.from_bytes(data[2:4], "little"),
                  "type": data[4]} if len(data) >= 5 else {}
        matches = len(data) >= 16 and header == bytes.fromhex("07 01 10 00 02 00 00 00")
    return {"received_bytes": len(data), "header_hex": header.hex(" "),
            "protocol_match": matches, "fields": fields}


def udp_probe(source, probe, host="p.njupt.edu.cn", timeout=10):
    source = str(ipaddress.IPv4Address(source))
    if source == "0.0.0.0" or timeout <= 0:
        raise ValueError("a concrete source IPv4 and positive timeout are required")
    port, payload = UDP_PROBES[probe]
    addresses = sorted({row[4][0] for row in socket.getaddrinfo(host, port, socket.AF_INET, socket.SOCK_DGRAM)})
    if len(addresses) != 1:
        raise ValueError("select a single IPv4 destination for this UDP observation")
    target = addresses[0], port
    result = {"observed_at": datetime.now(timezone.utc).isoformat(timespec="seconds"),
              "probe": probe, "target": {"host": host, "ipv4": target[0], "port": port},
              "sent_datagrams": 0, "sent_bytes": len(payload)}
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as connection:
        connection.settimeout(timeout)
        connection.bind((source, 0))
        connection.connect(target)
        try:
            connection.send(payload)
            result["sent_datagrams"] = 1
            data = connection.recv(65535)
        except TimeoutError:
            result["state"] = "no_response"
        except OSError as exc:
            result.update(state="socket_error", error=type(exc).__name__, errno=exc.errno)
        else:
            peer = connection.getpeername()
            result.update(state="response", peer={"ipv4": peer[0], "port": peer[1]},
                          **udp_evidence(probe, data))
    return result


def scan_observation(path):
    root = ET.parse(path).getroot()
    if root.tag != "nmaprun":
        raise ValueError("expected Nmap XML")
    hosts = []
    for host in root.findall("host"):
        ports = []
        for port in host.findall("ports/port"):
            state = port.find("state")
            service = port.find("service")
            ports.append({"protocol": port.get("protocol"), "port": int(port.get("portid")),
                          "state": state.get("state") if state is not None else None,
                          "reason": state.get("reason") if state is not None else None,
                          "service": service.attrib if service is not None else None})
        hosts.append({"addresses": [item.attrib for item in host.findall("address")],
                      "ports": ports, "unlisted_ports": [item.attrib for item in host.findall("ports/extraports")]})
    return {"scanner": root.get("scanner"), "version": root.get("version"),
            "scan_ranges": [item.attrib for item in root.findall("scaninfo")], "hosts": hosts}


def inspect(link, endpoints):
    results = []
    for endpoint in endpoints:
        host = urlsplit(endpoint).hostname
        record = {"endpoint": endpoint, "resolved_ipv4": sorted({row[4][0] for row in socket.getaddrinfo(host, None, socket.AF_INET)})}
        try:
            response = link.request("GET", endpoint, redirects=False)
            record.update(response.summary())
            if response.status == 200 and "html" in response.headers.get("Content-Type", "").lower():
                encoding = "gb18030" if host == "p.njupt.edu.cn" else "utf-8"
                record["page"] = Page(response.text(encoding)).inventory(response.url)
        except (OSError, RuntimeError, ValueError) as exc:
            record["error"] = str(exc)
        results.append(record)
    return results


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source")
    parser.add_argument("--timeout", type=float, default=10)
    parser.add_argument("--output")
    parser.add_argument("--endpoint", action="append", help="explicit HTTP URL; default is known entrance matrix")
    parser.add_argument("--nmap-xml", help="read an independently produced XML scan without scanning")
    parser.add_argument("--udp", choices=UDP_PROBES, help="send one selected UDP probe instead of HTTP observations")
    parser.add_argument("--host", default="p.njupt.edu.cn", help="UDP destination hostname or IPv4")
    args = parser.parse_args()
    if not args.source and not args.nmap_xml:
        parser.error("provide --source for HTTP observations or --nmap-xml for scan analysis")
    if args.udp and (not args.source or args.endpoint):
        parser.error("--udp requires --source and selects its own protocol port")
    result = {}
    if args.udp:
        result["udp"] = udp_probe(args.source, args.udp, args.host, args.timeout)
        if args.output:
            directory = Path(args.output)
            directory.mkdir(parents=True, exist_ok=True)
            with (directory / "udp-summary.json").open("x", encoding="utf-8") as stream:
                json.dump(result["udp"], stream, ensure_ascii=False, indent=2)
    elif args.source:
        result["http_entrances"] = inspect(Link(args.source, args.timeout, args.output), args.endpoint or ENDPOINTS)
    if args.nmap_xml:
        result["scan"] = scan_observation(args.nmap_xml)
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()

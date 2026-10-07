import unittest
from unittest.mock import Mock, patch

from deployment import UDP_PROBES, udp_evidence, udp_probe


class UDPTests(unittest.TestCase):
    def test_probe_payloads(self):
        self.assertEqual(UDP_PROBES["ntp"], (123, b"\x1b" + bytes(47)))
        self.assertEqual(UDP_PROBES["drcom-challenge"], (61440, bytes.fromhex("07 01 08 00 01 00 00 00")))

    def test_source_bound_single_datagram_and_header_only_summary(self):
        connection = Mock()
        connection.recv.return_value = bytes.fromhex("07 01 10 00 02 00 00 00") + b"salt" + bytes(20)
        connection.getpeername.return_value = ("192.0.2.1", 61440)
        factory = Mock()
        factory.return_value.__enter__ = Mock(return_value=connection)
        factory.return_value.__exit__ = Mock(return_value=False)
        with patch("deployment.socket.socket", factory), patch("deployment.socket.getaddrinfo", return_value=[
            (2, 2, 17, "", ("192.0.2.1", 61440))
        ]):
            result = udp_probe("192.0.2.10", "drcom-challenge", "example.test", 2)
        connection.bind.assert_called_once_with(("192.0.2.10", 0))
        connection.send.assert_called_once_with(UDP_PROBES["drcom-challenge"][1])
        self.assertEqual(result["header_hex"], "07 01 10 00 02 00 00 00")
        self.assertEqual(result["received_bytes"], 32)
        self.assertEqual(result["fields"]["length_field"], 16)
        self.assertTrue(result["protocol_match"])
        self.assertNotIn("salt", str(result))

    def test_ntp_fields_and_short_response(self):
        result = udp_evidence("ntp", bytes.fromhex("24 03 04 fa") + bytes(44))
        self.assertTrue(result["protocol_match"])
        self.assertEqual(result["fields"], {"version": 4, "mode": 4, "leap": 0, "stratum": 3})
        self.assertFalse(udp_evidence("ntp", b"")["protocol_match"])
        self.assertFalse(udp_evidence("drcom-challenge", b"")["protocol_match"])

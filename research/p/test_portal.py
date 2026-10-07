from pathlib import Path
import unittest
from p.portal import decode_jsonp, portal_routes


class JSONPTests(unittest.TestCase):
    def test_offline_sample(self):
        data = (Path(__file__).parent / "samples" / "status-offline.jsonp").read_bytes()
        self.assertEqual(decode_jsonp(data, "dr1000")["result"], 0)

    def test_wrong_callback_rejected(self):
        with self.assertRaisesRegex(ValueError, "callback"):
            decode_jsonp(b'dr1({"result":1})', "dr2")

    def test_trailing_javascript_rejected(self):
        with self.assertRaises(ValueError):
            decode_jsonp(b'dr1({});alert(1)', "dr1")

    def test_array_is_not_object(self):
        with self.assertRaises(ValueError):
            decode_jsonp(b'dr1([])', "dr1")

    def test_dispatch_expansion_preserves_object_prefix(self):
        source = '''var visitor={auth:function(){this.jsonp('authByQRCode',{});},
            records:function(){this.doload('loadOnlineTerm');},
            jsonp:function(action){return page.portal_api+"visitor/"+action;}};
            var general={auth:function(){this.jsonp('authByRemote');},
            jsonp:function(action){return page.portal_api+"educae_join/"+action;}};'''
        self.assertEqual(portal_routes(source), ["educae_join/authByRemote", "visitor/authByQRCode", "visitor/loadOnlineTerm"])

    def test_record_and_conditional_login_expansion(self):
        source = "var url=(x? page.path : page.portal_api)+'login'; var records=page.portal_api+'page/'; records+='loadOnlineRecord';"
        self.assertEqual(portal_routes(source), ["login", "page/loadOnlineRecord"])

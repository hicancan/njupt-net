import unittest

from pages import Page, assignment, fields, routes


class PageTests(unittest.TestCase):
    def test_successful_controls(self):
        page = Page('''<form action="/Self/login/verify"><input name="account" value="example">
            <input type="hidden" name="checkcode" value="nonce"><input name="ignored" disabled>
            <input type="checkbox" name="off"><input type="checkbox" name="on" checked>
            <input type="submit" name="submit" value="Login"><button name="button">Submit</button>
            <select name="year"><option value="2025">2025</option><option selected value="2026">2026</option></select>
            <textarea name="text">a&amp;b</textarea></form>''')
        self.assertEqual(fields(page.form("/Self/login/verify")),
                         {"account": "example", "checkcode": "nonce", "on": "on", "year": "2026", "text": "a&b"})

    def test_inventory_omits_form_values_and_session(self):
        page = Page('<form action="/Self/login/verify;jsessionid=secret"><input name="password" value="secret"></form>'
                    '<script src="/Self/resources/site.js;jsessionid=secret"></script>')
        value = page.inventory("http://example.test/Self/login")
        self.assertNotIn("secret", str(value))
        self.assertEqual(value["forms"][0]["fields"], [{"name": "password", "type": "input"}])

    def test_dynamic_portal_branch_literal(self):
        self.assertIn("login", routes("var url = (page.login_method==0 ? page.path : page.portal_api) + 'login';"))
        self.assertIn("getMacList", routes('url: "getMacList"'))

    def test_assignment_does_not_execute(self):
        self.assertEqual(assignment("var value='literal';", "value"), "literal")
        with self.assertRaises(ValueError):
            assignment("var value=fetch('https://example.test');", "value")

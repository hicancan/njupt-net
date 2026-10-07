"""Inspect HTML controls and literal script references; never execute scripts."""

from dataclasses import dataclass, field
from html.parser import HTMLParser
import re
from urllib.parse import urljoin, urlsplit


@dataclass
class Node:
    tag: str
    attrs: dict = field(default_factory=dict)
    children: list = field(default_factory=list)

    def walk(self):
        yield self
        for child in self.children:
            if isinstance(child, Node):
                yield from child.walk()

    def find(self, tag=None, **attrs):
        return next((n for n in self.walk() if (tag is None or n.tag == tag)
                     and all(n.attrs.get(k) == v for k, v in attrs.items())), None)

    def text(self):
        return "".join(child.text() if isinstance(child, Node) else child for child in self.children)


class Page(HTMLParser):
    VOID = {"area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr"}

    def __init__(self, text):
        super().__init__(convert_charrefs=True)
        self.root = Node("document")
        self.stack = [self.root]
        self.feed(text)
        self.close()

    def handle_starttag(self, tag, attrs):
        node = Node(tag, dict(attrs))
        self.stack[-1].children.append(node)
        if tag not in self.VOID:
            self.stack.append(node)

    def handle_startendtag(self, tag, attrs):
        self.handle_starttag(tag, attrs)
        if tag not in self.VOID:
            self.handle_endtag(tag)

    def handle_endtag(self, tag):
        for index in range(len(self.stack) - 1, 0, -1):
            if self.stack[index].tag == tag:
                del self.stack[index:]
                return

    def handle_data(self, text):
        self.stack[-1].children.append(text)

    @property
    def script(self):
        return "\n".join(n.text() for n in self.root.walk() if n.tag == "script" and not n.attrs.get("src"))

    def form(self, action):
        return next((n for n in self.root.walk() if n.tag == "form" and
                     re.sub(r";jsessionid=[^/;?]*", "", urlsplit(n.attrs.get("action", "")).path) == action), None)

    def inventory(self, base):
        forms, navigation, scripts = [], set(), set()
        for n in self.root.walk():
            if n.tag == "form":
                forms.append({"action": urlsplit(urljoin(base, n.attrs.get("action", ""))).path.split(";", 1)[0],
                              "method": n.attrs.get("method", "get").upper(),
                              "fields": [{"name": c.attrs["name"], "type": c.attrs.get("type", c.tag)}
                                         for c in n.walk() if c.attrs.get("name") and c.tag in {"input", "select", "textarea", "button"}]})
            if n.tag in {"a", "iframe"}:
                target = n.attrs.get("href" if n.tag == "a" else "src", "")
                if target and not target.startswith(("javascript:", "#")):
                    u = urlsplit(urljoin(base, target))
                    navigation.add(f"{u.scheme}://{u.netloc}{u.path.split(';', 1)[0]}")
            if n.tag == "script" and n.attrs.get("src"):
                scripts.add(re.sub(r";jsessionid=[^/;?]*", "", urljoin(base, n.attrs["src"])))
        return {"forms": forms, "navigation": sorted(navigation), "scripts": sorted(scripts),
                "script_routes": routes(self.script)}


def fields(form):
    if form is None:
        raise ValueError("expected form is absent")
    values = {}
    for n in form.walk():
        name = n.attrs.get("name")
        if not name or "disabled" in n.attrs:
            continue
        if n.tag == "input":
            kind = n.attrs.get("type", "text").lower()
            if kind in {"submit", "button", "reset", "image", "file"} or kind in {"checkbox", "radio"} and "checked" not in n.attrs:
                continue
            values[name] = n.attrs.get("value", "on" if kind in {"checkbox", "radio"} else "")
        elif n.tag == "textarea":
            values[name] = n.text()
        elif n.tag == "select":
            options = [c for c in n.walk() if c.tag == "option" and "disabled" not in c.attrs]
            selected = next((c for c in options if "selected" in c.attrs), options[0] if options else None)
            if selected:
                values[name] = selected.attrs.get("value", selected.text())
    return values


def assignment(text, name):
    pattern = rf"(?:^|[;\r\n])\s*(?:var\s+)?{re.escape(name)}\s*=\s*(?:'([^'\\]*)'|\"([^\"\\]*)\"|([0-9]+))\s*(?:[;,]|$)"
    match = re.search(pattern, text)
    if not match:
        raise ValueError(f"missing literal {name}")
    return next((value for value in match.groups() if value is not None), "")


def routes(script):
    """Static candidates, not a claim that a server implements each route."""
    absolute = re.findall(r"['\"]((?:/Self/|/eportal/portal/|/drcom/)[^'\"\s<>?]*)", script)
    relative = re.findall(r"(?:portal_path\s*\+|portal_api\s*\)?\s*\+|jsonp\s*\()\s*['\"]([^'\"\s?]+)", script)
    table_urls = re.findall(r"\burl\s*:\s*['\"]([^'\"\s<>?]+)['\"]", script)
    return sorted(set(absolute + relative + table_urls))

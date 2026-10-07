package zfw

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

type serverUser struct {
	Account      string      `json:"userName"`
	ConsumeLimit json.Number `json:"installmentFlag"`
}

var userObjectPattern = regexp.MustCompile(`\}\)\s*\(\s*(\{[^\r\n]+\})\s*\)\s*;`)

// The server embeds its user model as an IIFE argument. This model contains a
// password property; callers must select business fields rather than return it.
func selfUser(doc *html.Node) (*serverUser, error) {
	for _, match := range userObjectPattern.FindAllStringSubmatch(scriptText(doc), -1) {
		decoder := json.NewDecoder(strings.NewReader(match[1]))
		decoder.UseNumber()
		var user serverUser
		if decoder.Decode(&user) == nil {
			if user.Account != "" {
				return &user, nil
			}
		}
	}
	return nil, fmt.Errorf("Self page is missing its server-rendered user model")
}

func attr(n *html.Node, key string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func hasClass(n *html.Node, class string) bool {
	for _, item := range strings.Fields(attr(n, "class")) {
		if item == class {
			return true
		}
	}
	return false
}

func walk(n *html.Node, visit func(*html.Node)) {
	if n == nil {
		return
	}
	visit(n)
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		walk(child, visit)
	}
}

func find(n *html.Node, match func(*html.Node) bool) *html.Node {
	if n == nil {
		return nil
	}
	if match(n) {
		return n
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if result := find(child, match); result != nil {
			return result
		}
	}
	return nil
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var visit func(*html.Node)
	visit = func(current *html.Node) {
		if current == nil || (current.Type == html.ElementNode && (current.Data == "script" || current.Data == "style")) {
			return
		}
		if current.Type == html.TextNode {
			b.WriteString(current.Data)
			b.WriteByte(' ')
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

func scriptText(doc *html.Node) string {
	var text strings.Builder
	walk(doc, func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "script" && attr(n, "src") == "" {
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == html.TextNode {
					text.WriteString(child.Data)
				}
			}
			text.WriteByte('\n')
		}
	})
	return text.String()
}

func formValues(form *html.Node) url.Values {
	values := url.Values{}
	walk(form, func(n *html.Node) {
		if n.Type != html.ElementNode || n.Data != "input" || hasAttr(n, "disabled") || attr(n, "name") == "" {
			return
		}
		switch strings.ToLower(attr(n, "type")) {
		case "submit", "reset", "button", "image", "file":
			return
		case "checkbox", "radio":
			if !hasAttr(n, "checked") {
				return
			}
		}
		values.Add(attr(n, "name"), attr(n, "value"))
	})
	return values
}

func cloneValues(values url.Values) url.Values {
	result := make(url.Values, len(values))
	for key, entries := range values {
		result[key] = append([]string(nil), entries...)
	}
	return result
}

func selfNonce() string { return strconv.FormatFloat(rand.Float64(), 'f', -1, 64) }

func findForm(doc *html.Node, action string) *html.Node {
	return find(doc, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "form" && pathWithoutSession(attr(n, "action")) == action
	})
}

var feedbackPattern = regexp.MustCompile(`(?s)\(function\s*\(msg\)\s*\{.*?\}\)\('((?:\\.|[^'])*)'\);`)
var feedbackStatePattern = regexp.MustCompile(`\bvar\s+state\s*=\s*"([^"]*)"\s*;`)

func formFeedback(doc *html.Node) (string, string, error) {
	script := scriptText(doc)
	match := feedbackPattern.FindStringSubmatch(script)
	if len(match) != 2 {
		return "", "", fmt.Errorf("Self operation result is missing its feedback message")
	}
	message, err := unquoteJSString(match[1])
	if err != nil {
		return "", "", fmt.Errorf("Self operation feedback is not a valid string")
	}
	state := ""
	if match := feedbackStatePattern.FindStringSubmatch(script); len(match) == 2 {
		state = match[1]
	}
	return message, state, nil
}

func unquoteJSString(value string) (string, error) {
	var text strings.Builder
	for value != "" {
		r, _, rest, err := strconv.UnquoteChar(value, '\'')
		if err != nil {
			return "", err
		}
		text.WriteRune(r)
		value = rest
	}
	return text.String(), nil
}

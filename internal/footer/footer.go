package footer

import (
	stdhtml "html"
	"html/template"
	"io"
	"net/url"
	"strings"

	xhtml "golang.org/x/net/html"
)

var allowedTags = map[string]struct{}{
	"a":      {},
	"br":     {},
	"code":   {},
	"em":     {},
	"small":  {},
	"span":   {},
	"strong": {},
}

var dropSubtreeTags = map[string]struct{}{
	"iframe": {},
	"math":   {},
	"object": {},
	"script": {},
	"style":  {},
	"svg":    {},
}

func BindTemplateData(m map[string]any, raw string) {
	if m == nil {
		return
	}
	m["OptionFooter"] = raw
	m["RenderedFooter"] = template.HTML(Sanitize(raw))
}

func Sanitize(input string) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}
	root, err := parseSanitizeRoot(input)
	if err != nil {
		return stdhtml.EscapeString(input)
	}
	var b strings.Builder
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		renderSanitizedNode(&b, child)
	}
	return b.String()
}

func parseSanitizeRoot(input string) (*xhtml.Node, error) {
	const markerID = "__superflare_footer_sanitize_root__"
	doc, err := xhtml.Parse(strings.NewReader(`<!doctype html><html><body><div id="` + markerID + `">` + input + `</div></body></html>`))
	if err != nil {
		return nil, err
	}
	root := findNode(doc, func(node *xhtml.Node) bool {
		if node == nil || node.Type != xhtml.ElementNode || node.Data != "div" {
			return false
		}
		for _, attr := range node.Attr {
			if strings.EqualFold(attr.Key, "id") && attr.Val == markerID {
				return true
			}
		}
		return false
	})
	if root == nil {
		return nil, io.EOF
	}
	return root, nil
}

func findNode(node *xhtml.Node, match func(*xhtml.Node) bool) *xhtml.Node {
	if node == nil {
		return nil
	}
	if match(node) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findNode(child, match); found != nil {
			return found
		}
	}
	return nil
}

func renderSanitizedNode(b *strings.Builder, node *xhtml.Node) {
	if node == nil {
		return
	}

	switch node.Type {
	case xhtml.TextNode:
		b.WriteString(stdhtml.EscapeString(node.Data))
	case xhtml.ElementNode:
		tag := strings.ToLower(strings.TrimSpace(node.Data))
		if tag == "" {
			return
		}
		if _, drop := dropSubtreeTags[tag]; drop {
			return
		}
		if _, allowed := allowedTags[tag]; !allowed {
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				renderSanitizedNode(b, child)
			}
			return
		}

		if tag == "br" {
			b.WriteString("<br>")
			return
		}

		b.WriteByte('<')
		b.WriteString(tag)
		appendAllowedAttrs(b, tag, node.Attr)
		b.WriteByte('>')

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			renderSanitizedNode(b, child)
		}

		b.WriteString("</")
		b.WriteString(tag)
		b.WriteByte('>')
	default:
		return
	}
}

func appendAllowedAttrs(b *strings.Builder, tag string, attrs []xhtml.Attribute) {
	if tag != "a" {
		return
	}

	href := ""
	title := ""
	targetBlank := false
	for _, attr := range attrs {
		key := strings.ToLower(strings.TrimSpace(attr.Key))
		value := strings.TrimSpace(attr.Val)
		switch key {
		case "href":
			if safeHref, ok := sanitizeLinkHref(value); ok {
				href = safeHref
			}
		case "title":
			if value != "" {
				title = value
			}
		case "target":
			if strings.EqualFold(value, "_blank") {
				targetBlank = true
			}
		}
	}

	if href == "" {
		return
	}
	b.WriteString(` href="`)
	b.WriteString(stdhtml.EscapeString(href))
	b.WriteByte('"')
	if title != "" {
		b.WriteString(` title="`)
		b.WriteString(stdhtml.EscapeString(title))
		b.WriteByte('"')
	}
	if targetBlank {
		b.WriteString(` target="_blank" rel="noopener noreferrer"`)
	}
}

func sanitizeLinkHref(input string) (string, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", false
	}
	if strings.ContainsAny(input, "\x00\r\n\t") {
		return "", false
	}
	parsed, err := url.Parse(input)
	if err != nil {
		return "", false
	}
	if parsed.Scheme == "" {
		if strings.HasPrefix(input, "//") {
			return "", false
		}
		return input, true
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "mailto", "tel":
		return input, true
	default:
		return "", false
	}
}

package sanitize

import (
	"fmt"
	"strings"

	"ankiced/internal/domain"

	"golang.org/x/net/html"
)

var allow = map[string]bool{
	"b":      true,
	"i":      true,
	"u":      true,
	"strong": true,
	"span":   true,
	"div":    true,
	"br":     true,
	"img":    true,
}

var allowedImageAttrs = map[string]bool{
	"src":    true,
	"alt":    true,
	"title":  true,
	"width":  true,
	"height": true,
}

// allowedImageSchemes whitelists URL schemes that may appear inside an
// <img src="..."> attribute. Anki collections typically use either bare
// filenames (which are relative to the media folder, no scheme) or
// inline base64 data URIs, but we also accept http/https for completeness.
// Other schemes (`javascript:`, `vbscript:`, `file:`, ...) are rejected to
// shrink the attack surface when the cleaned HTML is later rendered in a
// browser/iframe context.
var allowedImageSchemes = map[string]bool{
	"http":  true,
	"https": true,
	"data":  true,
}

type HTMLCleanerTemplate struct{}

func (HTMLCleanerTemplate) ID() string { return domain.DefaultActionTemplateID }
func (HTMLCleanerTemplate) Name() string {
	return "HTML Cleaner"
}
func (HTMLCleanerTemplate) Apply(input string) (string, error) {
	return keepBasicTags(input)
}

type TemplateRegistry struct {
	templates map[string]domain.ActionTemplate
	defaultID string
}

func NewTemplateRegistry() TemplateRegistry {
	htmlCleaner := HTMLCleanerTemplate{}
	return TemplateRegistry{
		templates: map[string]domain.ActionTemplate{
			htmlCleaner.ID(): htmlCleaner,
		},
		defaultID: htmlCleaner.ID(),
	}
}

func (r TemplateRegistry) Get(id string) (domain.ActionTemplate, error) {
	template, ok := r.templates[strings.TrimSpace(id)]
	if !ok {
		return nil, fmt.Errorf("action template not found: %s", id)
	}
	return template, nil
}

func (r TemplateRegistry) Default() (domain.ActionTemplate, error) {
	return r.Get(r.defaultID)
}

func keepBasicTags(input string) (string, error) {
	nodes, err := html.ParseFragment(strings.NewReader(input), nil)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, n := range nodes {
		writeNode(&b, n)
	}
	return b.String(), nil
}

func writeNode(b *strings.Builder, n *html.Node) {
	switch n.Type {
	case html.TextNode:
		// The HTML parser decodes entities (e.g. &amp; -> &). Re-encode them
		// here so the output stays valid HTML and round-trips cleanly when
		// rendered or re-parsed downstream.
		b.WriteString(html.EscapeString(n.Data))
	case html.ElementNode:
		if allow[n.Data] {
			if n.Data == "br" {
				b.WriteString("<br>")
				return
			}
			if n.Data == "img" {
				writeImage(b, n)
				return
			}
			b.WriteString("<")
			b.WriteString(n.Data)
			b.WriteString(">")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				writeNode(b, c)
			}
			b.WriteString("</")
			b.WriteString(n.Data)
			b.WriteString(">")
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			writeNode(b, c)
		}
	default:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			writeNode(b, c)
		}
	}
}

func writeImage(b *strings.Builder, n *html.Node) {
	b.WriteString("<img")
	for _, attr := range n.Attr {
		key := strings.ToLower(strings.TrimSpace(attr.Key))
		val := strings.TrimSpace(attr.Val)
		if !allowedImageAttrs[key] || val == "" {
			continue
		}
		// `src` must use either no scheme (bare media filename) or a
		// vetted scheme. Other URLs are dropped to avoid `javascript:`
		// and similar exfil/XSS vectors when the sanitised HTML is
		// rendered in a browser/iframe.
		if key == "src" && !isSafeImageSrc(val) {
			continue
		}
		b.WriteString(" ")
		b.WriteString(key)
		b.WriteString(`="`)
		b.WriteString(html.EscapeString(attr.Val))
		b.WriteString(`"`)
	}
	b.WriteString(">")
}

// isSafeImageSrc returns true when val is either a bare path (no URL
// scheme) or carries a scheme present in allowedImageSchemes. Schemes are
// matched case-insensitively per RFC 3986 section 3.1.
func isSafeImageSrc(val string) bool {
	idx := strings.IndexByte(val, ':')
	if idx <= 0 {
		// No scheme present → relative URL or bare media filename.
		return true
	}
	// A leading slash before the first ':' (e.g. `/path:weird`) is a
	// path, not a scheme; permit it without further checks.
	if strings.ContainsAny(val[:idx], "/?#") {
		return true
	}
	scheme := strings.ToLower(val[:idx])
	return allowedImageSchemes[scheme]
}

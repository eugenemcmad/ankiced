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
		b.WriteString(n.Data)
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
		if !allowedImageAttrs[key] || strings.TrimSpace(attr.Val) == "" {
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

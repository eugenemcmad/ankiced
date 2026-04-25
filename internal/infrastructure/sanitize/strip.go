package sanitize

import (
	"strings"

	"golang.org/x/net/html"
)

// StripAllTags returns the plain-text content of the given HTML fragment.
// On parse failure it logs nothing and returns the raw input — callers can wrap
// this for richer reporting if needed.
func StripAllTags(input string) string {
	if input == "" {
		return ""
	}
	nodes, err := html.ParseFragment(strings.NewReader(input), nil)
	if err != nil {
		return input
	}
	var b strings.Builder
	for _, n := range nodes {
		writePlainText(&b, n)
	}
	return b.String()
}

func writePlainText(b *strings.Builder, n *html.Node) {
	if n.Type == html.TextNode {
		b.WriteString(n.Data)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		writePlainText(b, c)
	}
}

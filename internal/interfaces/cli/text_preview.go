package cli

import (
	"strings"

	"golang.org/x/net/html"
)

func stripAllTags(input string) string {
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

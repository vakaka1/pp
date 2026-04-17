package ppfallback

import (
	stdhtml "html"
	"strings"

	"golang.org/x/net/html"
)

var articleBodyHints = []string{
	"post-content-body",
	"tm-article-body",
	"article-formatted-body",
	"tm-article-presenter__body",
	"article-body",
}

func extractArticleTextFromHTML(doc *html.Node) string {
	root := findArticleRoot(doc)
	if root == nil {
		root = doc
	}

	blocks := make([]string, 0, 32)
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil {
			return
		}
		if shouldSkipNode(node) {
			return
		}
		if isTextBlock(node) {
			block := normalizeBlockText(extractInlineText(node))
			if block != "" {
				blocks = append(blocks, block)
			}
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)

	if len(blocks) == 0 {
		return normalizeBlockText(extractInlineText(root))
	}

	return strings.Join(dedupeAdjacent(blocks), "\n\n")
}

func findArticleRoot(node *html.Node) *html.Node {
	var articleFallback *html.Node
	var walk func(*html.Node) *html.Node
	walk = func(current *html.Node) *html.Node {
		if current == nil {
			return nil
		}
		if hasArticleBodyHint(current) {
			return current
		}
		if articleFallback == nil && current.Type == html.ElementNode && current.Data == "article" {
			articleFallback = current
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			if found := walk(child); found != nil {
				return found
			}
		}
		return nil
	}

	if found := walk(node); found != nil {
		return found
	}
	return articleFallback
}

func hasArticleBodyHint(node *html.Node) bool {
	if node == nil || node.Type != html.ElementNode {
		return false
	}
	for _, attr := range node.Attr {
		switch attr.Key {
		case "id", "class":
			for _, hint := range articleBodyHints {
				if strings.Contains(attr.Val, hint) {
					return true
				}
			}
		}
	}
	return false
}

func shouldSkipNode(node *html.Node) bool {
	if node == nil || node.Type != html.ElementNode {
		return false
	}

	switch node.Data {
	case "script", "style", "noscript", "svg", "img", "picture", "source", "figure", "video", "iframe", "form", "button":
		return true
	}

	for _, attr := range node.Attr {
		if attr.Key == "class" && (strings.Contains(attr.Val, "banner") || strings.Contains(attr.Val, "advert") || strings.Contains(attr.Val, "tm-sticky-column")) {
			return true
		}
	}

	return false
}

func isTextBlock(node *html.Node) bool {
	if node == nil || node.Type != html.ElementNode {
		return false
	}

	switch node.Data {
	case "p", "li", "blockquote", "pre", "h1", "h2", "h3", "h4", "h5", "h6":
		return true
	}
	return false
}

func extractInlineText(node *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current == nil {
			return
		}
		if shouldSkipNode(current) {
			return
		}

		switch current.Type {
		case html.TextNode:
			b.WriteString(current.Data)
		case html.ElementNode:
			if current.Data == "br" {
				b.WriteString("\n")
				return
			}
			for child := current.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
		default:
			for child := current.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
		}
	}

	walk(node)
	return b.String()
}

func normalizeBlockText(raw string) string {
	raw = stdhtml.UnescapeString(raw)
	raw = strings.ReplaceAll(raw, "\u00a0", " ")
	raw = strings.ReplaceAll(raw, "\r", "\n")

	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			out = append(out, line)
		}
	}

	return strings.TrimSpace(strings.Join(out, "\n"))
}

func dedupeAdjacent(blocks []string) []string {
	if len(blocks) == 0 {
		return nil
	}

	out := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if len(out) > 0 && out[len(out)-1] == block {
			continue
		}
		out = append(out, block)
	}
	return out
}

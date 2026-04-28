package ppfallback

import (
	stdhtml "html"
	"net/url"
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
	return extractArticleTextFromHTMLWithBase(doc, "")
}

func extractArticleTextFromHTMLWithBase(doc *html.Node, baseURL string) string {
	root := findArticleRoot(doc)
	if root == nil {
		root = doc
	}

	blocks := make([]string, 0, 32)
	stopAtMetadata := false
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil || stopAtMetadata {
			return
		}
		if shouldSkipNode(node) {
			return
		}
		if block := imageMarkdownBlock(node, baseURL); block != "" {
			blocks = append(blocks, block)
			return
		}
		if isTextBlock(node) {
			block := normalizeBlockText(extractInlineMarkdown(node, baseURL))
			if isTrailingArticleMetadataBlock(block) {
				stopAtMetadata = true
				return
			}
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
		return normalizeBlockText(extractInlineMarkdown(root, baseURL))
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
	case "script", "style", "noscript", "svg", "video", "iframe", "form", "button":
		return true
	}

	for _, attr := range node.Attr {
		if attr.Key != "class" && attr.Key != "id" {
			continue
		}
		value := strings.ToLower(attr.Val)
		for _, marker := range []string{
			"banner",
			"advert",
			"tm-sticky-column",
			"tm-article-snippet__hubs",
			"tm-tags-list",
			"tm-article-presenter__footer",
			"tm-article-presenter__meta",
			"tm-votes",
			"tm-user-info",
			"tm-comment",
		} {
			if strings.Contains(value, marker) {
				return true
			}
		}
	}

	return false
}

func isTextBlock(node *html.Node) bool {
	if node == nil || node.Type != html.ElementNode {
		return false
	}

	switch node.Data {
	case "p", "li", "blockquote", "pre", "h2", "h3", "h4", "h5", "h6":
		return true
	}
	return false
}

func imageMarkdownBlock(node *html.Node, baseURL string) string {
	image := firstImageNode(node)
	if image == nil {
		return ""
	}

	src := resolveArticleURL(firstNonEmptyAttr(image, "data-src", "src"), baseURL)
	if src == "" {
		return ""
	}

	alt := sanitizeMarkdownLabel(firstNonEmptyAttr(image, "alt", "title"))
	return "![" + alt + "](" + src + ")"
}

func firstImageNode(node *html.Node) *html.Node {
	if node == nil {
		return nil
	}
	if node.Type == html.ElementNode && node.Data == "img" {
		return node
	}
	if node.Type == html.ElementNode && node.Data != "figure" && node.Data != "picture" {
		return nil
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := firstImageNode(child); found != nil {
			return found
		}
	}
	return nil
}

func firstNonEmptyAttr(node *html.Node, keys ...string) string {
	if node == nil {
		return ""
	}
	for _, key := range keys {
		for _, attr := range node.Attr {
			if attr.Key == key && strings.TrimSpace(attr.Val) != "" {
				return strings.TrimSpace(attr.Val)
			}
		}
	}
	return ""
}

func extractInlineMarkdown(node *html.Node, baseURL string) string {
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
			switch current.Data {
			case "br":
				b.WriteString("\n")
				return
			case "img", "figure", "picture":
				return
			case "a":
				label := normalizeBlockText(extractPlainInlineText(current))
				href := resolveArticleURL(firstNonEmptyAttr(current, "href"), baseURL)
				if label == "" {
					return
				}
				if href == "" {
					b.WriteString(label)
					return
				}
				b.WriteString("[")
				b.WriteString(sanitizeMarkdownLabel(label))
				b.WriteString("](")
				b.WriteString(href)
				b.WriteString(")")
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

func extractPlainInlineText(node *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current == nil || shouldSkipNode(current) {
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
			if current.Data == "img" || current.Data == "figure" || current.Data == "picture" {
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

func resolveArticleURL(rawURL string, baseURL string) string {
	rawURL = strings.TrimSpace(stdhtml.UnescapeString(rawURL))
	if rawURL == "" || strings.HasPrefix(rawURL, "#") {
		return ""
	}
	if strings.HasPrefix(rawURL, "//") {
		return "https:" + rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if parsed.IsAbs() {
		if parsed.Scheme == "http" || parsed.Scheme == "https" {
			return parsed.String()
		}
		return ""
	}
	if strings.TrimSpace(baseURL) == "" {
		return ""
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	resolved := base.ResolveReference(parsed)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return ""
	}
	return resolved.String()
}

func sanitizeMarkdownLabel(label string) string {
	label = strings.Join(strings.Fields(label), " ")
	replacer := strings.NewReplacer("[", " ", "]", " ", "(", " ", ")", " ")
	label = replacer.Replace(label)
	return strings.Join(strings.Fields(label), " ")
}

func isTrailingArticleMetadataBlock(block string) bool {
	normalized := strings.Trim(strings.ToLower(strings.Join(strings.Fields(block), " ")), " :")
	return normalized == "теги" ||
		normalized == "хабы" ||
		strings.HasPrefix(normalized, "теги:") ||
		strings.HasPrefix(normalized, "хабы:")
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

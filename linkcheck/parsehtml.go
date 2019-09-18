package linkcheck

import (
	"bytes"
	"net/url"
	"regexp"

	"golang.org/x/net/html"
)

func getLinks(pageurl *url.URL, body []byte) (links []string, err error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	visitAll(doc, func(n *html.Node) {
		if link := linkFromAHref(pageurl, n); link != "" {
			links = append(links, link)
		}
	})

	return links, nil
}

func visitAll(n *html.Node, callback func(*html.Node)) {
	callback(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		visitAll(c, callback)
	}
}

func linkFromAHref(pageurl *url.URL, n *html.Node) (link string) {
	if !isAnchor(n) {
		return
	}

	return resolveRef(pageurl, href(n))
}

func isAnchor(n *html.Node) bool {
	return n.Type == html.ElementNode && n.Data == "a"
}

func href(n *html.Node) string {
	for _, attr := range n.Attr {
		if attr.Key == "href" {
			return attr.Val
		}
	}
	return ""
}

func resolveRef(baseurl *url.URL, ref string) string {
	u, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	return baseurl.ResolveReference(u).String()
}

var idRx = regexp.MustCompile(`\bid=['"]?([^\s'">]+)`)

func pageIDs(body []byte) (ids []string) {
	mv := idRx.FindAllSubmatch(body, -1)
	for _, m := range mv {
		ids = append(ids, string(m[1]))
	}
	return
}

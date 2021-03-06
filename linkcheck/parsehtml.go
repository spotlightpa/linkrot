package linkcheck

import (
	"net/url"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func getIDsAndLinks(pageurl *url.URL, doc *html.Node, getLinks bool) (ids, links []string) {
	visitAll(doc, func(n *html.Node) {
		ids = append(ids, getIDs(n)...)
		if !getLinks {
			return
		}
		if link := linkFromAHref(pageurl, n); link != "" {
			links = append(links, link)
		}
	})

	return ids, links
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
	return n.Type == html.ElementNode && n.DataAtom == atom.A
}

func getIDs(n *html.Node) []string {
	var ids []string
	for _, attr := range n.Attr {
		if attr.Key == "id" {
			ids = append(ids, attr.Val)
		}
	}
	// collect old fashioned <a name=""> anchors
	if isAnchor(n) {
		for _, attr := range n.Attr {
			if attr.Key == "name" {
				ids = append(ids, attr.Val)
			}
		}
	}
	return ids
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

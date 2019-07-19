package linkcheck

import "net/url"

func removeFragment(link string) string {
	u, _ := url.Parse(link)
	u.Fragment = ""
	return u.String()
}

func splitFragment(linkIn string) (link, frag string) {
	u, _ := url.Parse(linkIn)
	frag = u.Fragment
	u.Fragment = ""
	link = u.String()
	return
}

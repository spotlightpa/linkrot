package linkcheck

import (
	"net/url"
	"sort"
	"strings"
)

// excludeLink globals
var (
	invalidProtos = []string{
		"mailto:",
		"javascript:",
		"tel:",
		"sms:",
	}
	excludePaths []string
)

func excludeLink(ref string) bool {
	for _, proto := range invalidProtos {
		if strings.HasPrefix(ref, proto) {
			return true
		}
	}
	for _, prefix := range excludePaths {
		if strings.HasPrefix(ref, prefix) {
			return true
		}
	}
	return false
}

// parses URL and resolves references
func parseURL(baseurl, ref string) string {
	base, _ := url.Parse(baseurl)
	u, err := url.Parse(ref)
	if err != nil {
		panic(err)
	}
	return base.ResolveReference(u).String()
}

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

func sliceToSet(ss []string) map[string]bool {
	set := make(map[string]bool, len(ss))
	for _, s := range ss {
		set[s] = true
	}
	return set
}

func setToSlice(set map[string]bool) []string {
	ss := make([]string, 0, len(set))
	for s := range set {
		ss = append(ss, s)
	}
	sort.Strings(ss)
	return ss
}

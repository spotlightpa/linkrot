package linkcheck

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
)

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

func statusCheck(resp *http.Response, acceptStatuses ...int) error {
	for _, code := range acceptStatuses {
		if resp.StatusCode == code {
			return nil
		}
	}

	return fmt.Errorf("unexpected status: %s", resp.Status)
}

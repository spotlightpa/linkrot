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

func statusReject(resp *http.Response, rejectStatuses ...int) error {
	for _, code := range rejectStatuses {
		if resp.StatusCode == code {
			return fmt.Errorf("bad status: %s", resp.Status)
		}
	}

	return nil
}

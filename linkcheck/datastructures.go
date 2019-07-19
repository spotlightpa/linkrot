package linkcheck

import "strings"

type queue struct {
	base string
	q    []string
	m    map[string]bool
}

func newQueue(url string) *queue {
	return &queue{
		q: []string{url},
		m: map[string]bool{url: true},
	}
}

func (q *queue) empty() bool {
	return len(q.q) == 0
}

func (q *queue) head() string {
	if q.empty() {
		return ""
	}
	return q.q[0]
}

func (q *queue) pophead() {
	if !q.empty() {
		q.q = q.q[1:]
	}
}

func (q *queue) add(link string) {
	link = removeFragment(link)
	// Only add if it's not queued before
	if _, seen := q.m[link]; seen {
		return
	}
	q.q = append(q.q, link)
	q.m[link] = true
}

type pageError struct {
	err              error
	refs             []string
	missingFragments map[string]bool
}

type urlErrors map[string]*pageError

// fetchResult is a type so that we can send fetch's results on a channel
type fetchResult struct {
	url   string
	links []string
	ids   []string
	err   error
}

type pageInfo struct {
	ids   map[string]bool
	links map[string]bool
	err   error
}

type crawledPages map[string]pageInfo

func newCrawledPages() crawledPages {
	return make(crawledPages)
}

func (cp crawledPages) add(fr fetchResult) {
	if fr.err != nil {
		cp[fr.url] = pageInfo{err: fr.err}
		return
	}
	pi := pageInfo{
		ids:   make(map[string]bool, len(fr.ids)),
		links: make(map[string]bool, len(fr.links)),
	}
	for _, link := range fr.links {
		pi.links[link] = true
	}
	for _, id := range fr.ids {
		pi.ids[id] = true
	}
	cp[fr.url] = pi
}

func (cp crawledPages) addLinksToQueue(url string, q *queue) {
	pi := cp[url]
	for link := range pi.links {
		q.add(link)
	}
}

func (cp crawledPages) toURLErrors(base string) urlErrors {
	requestErrs := make(urlErrors)
	// Put all errors into errs
	for url, pi := range cp {
		if pi.err != nil {
			requestErrs[url] = &pageError{pi.err, nil, nil}
		}
	}
	// For each page, if one of its links is in errs,
	// add that to the back refs and check for its
	// link ids in frags
	fragErrs := make(urlErrors)
	for page, pi := range cp {
		// ignore pages off site
		if !strings.HasPrefix(page, base) {
			continue
		}
		for link := range pi.links {
			link, frag := splitFragment(link)
			if pe, ok := requestErrs[link]; ok {
				pe.refs = append(pe.refs, page)
			}
			if frag == "" {
				continue
			}
			if target, ok := cp[link]; ok && target.ids[frag] {
				continue
			}
			// fragment was missing
			pe := fragErrs[link]
			if pe == nil {
				pe = &pageError{ErrMissingFragment, nil, make(map[string]bool)}
				fragErrs[link] = pe
			}
			pe.refs = append(pe.refs, page)
			pe.missingFragments[frag] = true
		}
	}
	// Merge errors
	for url, pe := range fragErrs {
		requestErrs[url] = pe
	}
	return requestErrs
}

// Original copyright/license below.
//
// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package linkcheck finds missing links in the given website.
// It crawls a URL recursively and notes URLs and URL fragments
// that it's seen and prints a report of missing links at the end.
package linkcheck

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"strings"
	"syscall"

	"github.com/carlmjohnson/exitcode"

	"golang.org/x/net/html"
)

var (
	ErrCancelled       = exitcode.Set(errors.New("scraping canceled by SIGINT"), 3)
	ErrBadLinks        = exitcode.Set(errors.New("found bad links"), 4)
	ErrMissingFragment = errors.New("page missing fragments")
)

func CLI(args []string) error {
	fl := flag.NewFlagSet("linkrot", flag.ContinueOnError)
	fl.Usage = func() {
		const usage = `Usage of linkrot:

linkcheck [options] <url>

    linkcheck takes a root URL and recurses down through the links it finds
    in the HTML pages, checking for broken links (HTTP status != 200).

Options:
`
		fmt.Fprintln(os.Stderr, usage)
		fl.PrintDefaults()
	}

	verbose := fl.Bool("verbose", false, "verbose")
	crawlers := fl.Int("crawlers", runtime.NumCPU(), "number of concurrent crawlers")
	excludes := fl.String("exclude", "", "comma separated list of URL prefixes to ignore")
	if err := fl.Parse(args); err != nil {
		return err
	}

	root := fl.Arg(0)
	if root == "" {
		root = "http://localhost:8000"
	}

	base, err := url.Parse(root)
	if err != nil {
		log.Printf("parsing root URL: %v", err)
		return err
	}

	if base.Path == "" {
		base.Path = "/"
	}

	if *crawlers < 1 {
		log.Printf("need at least one crawler")
		return fmt.Errorf("bad crawler count: %d", *crawlers)
	}

	logger := log.New(ioutil.Discard, "linkrot ", log.LstdFlags)
	if *verbose {
		logger = log.New(os.Stderr, "linkrot ", log.LstdFlags)
	}

	if *excludes != "" {
		excludePaths = strings.Split(*excludes, ",")
	}

	c := &crawler{base.String(), *crawlers, logger}
	return c.run(os.Stdout)
}

type crawler struct {
	base    string
	workers int
	*log.Logger
}

func (c *crawler) run(w io.Writer) error {
	errs, cancelled := c.crawl()

	// TODO: maybe output this as CSV or something?
	for url, err := range errs {
		fmt.Fprintf(w, "%s: %v\n", url, err.err)
	}

	var err error
	if cancelled {
		err = ErrCancelled
	} else if len(errs) > 0 {
		err = ErrBadLinks
	}

	return err
}

func (c *crawler) crawl() (errs urlErrors, cancelled bool) {
	c.Printf("starting %d crawlers", c.workers)

	var (
		workerqueue  = make(chan string)
		fetchResults = make(chan fetchResult)
	)

	for i := 0; i < c.workers; i++ {
		go func() {
			for url := range workerqueue {
				fetchResults <- c.fetch(url)
			}
		}()
	}

	var (
		// List of URLs that need to be crawled
		q = newQueue(c.base)
		// database of what we've collected
		crawled = newCrawledPages()
		// How many fetches we're waiting on
		openFetchs int
	)

	// subscribe to SIGINT signals, so that we still output on early exit
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, syscall.SIGINT)

	for (openFetchs > 0 || !q.empty()) && !cancelled {
		loopqueue := workerqueue
		addURL := q.head()
		if q.empty() {
			loopqueue = nil
		}

		select {
		// This case is a NOOP when queue is empty
		// because loopqueue will be nil and nil always blocks
		case loopqueue <- addURL:
			openFetchs++
			q.pophead()

		case result := <-fetchResults:
			openFetchs--
			crawled.add(result)
			// Only queue links on pages under root
			if strings.HasPrefix(result.url, c.base) {
				crawled.addLinksToQueue(result.url, q)
			}

		case <-stopChan:
			cancelled = true
		}
	}

	// Fetched everything!
	close(workerqueue)

	return crawled.toURLErrors(c.base), cancelled
}

func (c *crawler) fetch(url string) fetchResult {
	processLinks := strings.HasPrefix(url, c.base)
	links, ids, err := c.processURL(url, processLinks)
	return fetchResult{url, links, ids, err}
}

func (c *crawler) processURL(url string, processLinks bool) (links, ids []string, err error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, nil, err
	}

	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, nil, errors.New(res.Status)
	}

	buf := bufio.NewReader(res.Body)
	// http.DetectContentType only uses first 512 bytes
	peek, err := buf.Peek(512)
	if err != nil && err != io.EOF {
		c.Printf("Error initially reading %s body: %v", url, err)
		return nil, nil, err
	}

	if ct := http.DetectContentType(peek); !strings.HasPrefix(ct, "text/html") {
		c.Printf("Skipping %s, content-type %s", url, ct)
		return nil, nil, nil
	}

	slurp, err := ioutil.ReadAll(buf)
	if err != nil {
		c.Printf("Error reading %s body: %v", url, err)
		return nil, nil, err
	}

	c.Println("Got OK:", url)

	if processLinks {
		for _, ref := range c.getLinks(url, slurp) {
			c.Printf("url %s links to %s", url, ref)

			if !excludeLink(ref) {
				links = append(links, ref)
			}
		}
	}

	ids = pageIDs(slurp)
	return links, ids, nil
}

func (c *crawler) getLinks(url string, body []byte) (links []string) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		c.Printf("error parsing HTML: %v", err)
		// TODO: Should we return an error here?
		return
	}

	var f func(*html.Node)
	f = func(n *html.Node) {
		if isAnchor(n) {
			ref := href(n)
			ref = parseURL(url, ref)
			links = append(links, ref)
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	return
}

var idRx = regexp.MustCompile(`\bid=['"]?([^\s'">]+)`)

func pageIDs(body []byte) (ids []string) {
	mv := idRx.FindAllSubmatch(body, -1)
	for _, m := range mv {
		ids = append(ids, string(m[1]))
	}
	return
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

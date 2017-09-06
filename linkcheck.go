// Modifications 2017 by Ad Hoc. Original copyright/license below.
//
// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The linkcheck command finds missing links in the given website.
// It crawls a URL recursively and notes URLs and URL fragments
// that it's seen and prints a report of missing links at the end.
package main

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

	"golang.org/x/net/html"
)

func main() {
	flag.Usage = func() {
		const usage = `Usage of linkcheck:

linkcheck [options] <url>

    linkcheck takes a root URL and recurses down through the links it finds
    in the HTML pages, checking for broken links (HTTP status != 200).

Options:
`
		fmt.Fprintln(os.Stderr, usage)
		flag.PrintDefaults()
	}

	verbose := flag.Bool("verbose", false, "verbose")
	crawlers := flag.Int("crawlers", runtime.NumCPU(), "number of concurrent crawlers")
	excludes := flag.String("exclude", "", "comma separated list of URL prefixes to ignore")
	flag.Parse()
	root := flag.Arg(0)
	if root == "" {
		root = "http://localhost:8000"
	}

	base, err := url.Parse(root)
	if err != nil {
		log.Fatalf("parsing root URL: %v", err)
	}

	if base.Path == "" {
		base.Path = "/"
	}

	if *crawlers < 1 {
		log.Fatalf("need at least one crawler")
	}

	if !*verbose {
		log.SetOutput(ioutil.Discard)
	}

	if *excludes != "" {
		excludePaths = strings.Split(*excludes, ",")
	}

	os.Exit(run(base.String(), *crawlers, os.Stdout))
}

// fetchResult is a type so that we can send fetch's results on a channel
type fetchResult struct {
	url   string
	links []string
	ids   map[string]bool
	err   error
}

// urlErr is an error plus the URL that was the source of the error
type urlErr struct {
	url string
	err error
}

func run(base string, crawlers int, output io.Writer) (exitCode int) {
	log.Printf("starting %d crawlers", crawlers)

	var (
		workerqueue  = make(chan string)
		fetchResults = make(chan fetchResult)
	)

	for i := 0; i < crawlers; i++ {
		go func() {
			for url := range workerqueue {
				processLinks := strings.HasPrefix(url, base)
				links, ids, err := fetch(url, processLinks)
				fetchResults <- fetchResult{url, links, ids, err}
			}
		}()
	}

	var (
		// URL that was fetched -> []URLs it links to
		needs = make(map[string][]string)
		// URL without fragment -> set of ids on page
		crawled = make(map[string]map[string]bool)
		// List of URLs that need to be crawled
		queue = []string{base}
		// Set of URLs that have been queued already
		queued = map[string]bool{base: true}
		// How many fetches we're waiting on
		openFetchs int
		// Any problems we encounter along the way
		errs []urlErr
	)

	// subscribe to SIGINT signals, so that we still output on early exit
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, syscall.SIGINT)

	for (openFetchs > 0 || len(queue) > 0) && exitCode == 0 {
		var loopqueue chan string
		addURL := ""
		if len(queue) > 0 {
			loopqueue = workerqueue
			addURL = queue[0]
		}

		select {
		// This case is a NOOP when queue is empty
		// because loopqueue will be nil and nil always blocks
		case loopqueue <- addURL:
			openFetchs++
			queue = queue[1:]

		case result := <-fetchResults:
			openFetchs--
			if result.err != nil {
				errs = append(errs, urlErr{result.url, result.err})
				break
			}
			crawled[result.url] = result.ids

			// Only queue links under root
			if !strings.HasPrefix(result.url, base) {
				break
			}

			needs[result.url] = result.links
			for _, link := range result.links {
				// Clear any fragments before queueing
				u, _ := url.Parse(link)
				u.Fragment = ""
				link = u.String()
				if !queued[link] {
					queued[link] = true
					queue = append(queue, link)
				}
			}

		case <-stopChan:
			exitCode = 3
		}
	}

	// Fetched everything!
	close(workerqueue)

	// Now check if it fulfilled our needs
	for srcURL, destURLs := range needs {
		for _, destURL := range destURLs {
			u, _ := url.Parse(destURL)
			frag := u.Fragment
			u.Fragment = ""
			link := u.String()
			if crawled[link] == nil {
				errs = append(errs, urlErr{
					srcURL,
					fmt.Errorf("failed to fetch: %s", destURL)})
			} else if frag != "" && !crawled[link][frag] {
				errs = append(errs, urlErr{
					srcURL,
					fmt.Errorf("missing fragment: %s", destURL)})
			}
		}
	}

	// TODO: maybe output this as CSV or something?
	for _, err := range errs {
		fmt.Fprintf(output, "%s: %v\n", err.url, err.err)
	}

	if len(errs) > 0 && exitCode == 0 {
		exitCode = 1
	}

	return exitCode
}

func fetch(url string, processLinks bool) (links []string, ids map[string]bool, err error) {
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
		log.Printf("Error initially reading %s body: %v", url, err)
		return nil, nil, err
	}

	if ct := http.DetectContentType(peek); !strings.HasPrefix(ct, "text/html") {
		log.Printf("Skipping %s, content-type %s", url, ct)
		// Have to make ID non-nil, so that it shows up in the map of
		// URLs we've crawled
		return nil, map[string]bool{}, nil
	}

	slurp, err := ioutil.ReadAll(buf)
	if err != nil {
		log.Printf("Error reading %s body: %v", url, err)
		return nil, nil, err
	}

	log.Println("Got OK:", url)

	if processLinks {
		for _, ref := range getLinks(url, slurp) {
			log.Printf("url %s links to %s", url, ref)

			if !excludeLink(ref) {
				links = append(links, ref)
			}
		}
	}

	ids = make(map[string]bool)
	for _, id := range pageIDs(slurp) {
		log.Printf(" url %s has #%s", url, id)
		ids[id] = true
	}

	return links, ids, nil
}

func getLinks(url string, body []byte) (links []string) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		log.Printf("error parsing HTML: %v", err)
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

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
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/carlmjohnson/exitcode"
	"github.com/carlmjohnson/flagext"
	sentry "github.com/getsentry/sentry-go"
	"golang.org/x/net/publicsuffix"
)

// Errors native to linkcheck
var (
	ErrCancelled       = exitcode.Set(errors.New("scraping canceled by SIGINT"), 3)
	ErrBadLinks        = exitcode.Set(errors.New("found bad links"), 4)
	ErrMissingFragment = errors.New("page missing fragments")
)

const (
	chromeUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/71.0.3578.98 Safari/537.36"
)

// CLI runs the linkrot executable, equivalent to calling it on the command line.
func CLI(args []string) error {
	fl := flag.NewFlagSet("linkrot", flag.ContinueOnError)
	fl.Usage = func() {
		const usage = `Usage of linkrot:

linkrot [options] <url>

    linkrot takes a root URL and recurses down through the links it finds
    in the HTML pages, checking for broken links (HTTP status != 200).

    Options may also be specified as env vars prefixed with "LINKROT_".

Options:
`
		fmt.Fprintln(os.Stderr, usage)
		fl.PrintDefaults()
	}

	verbose := fl.Bool("verbose", false, "verbose")
	crawlers := fl.Int("crawlers", runtime.NumCPU(), "number of concurrent crawlers")
	timeout := fl.Duration("timeout", 10*time.Second, "timeout for requesting a URL")
	var excludePaths []string
	fl.Func("exclude", "`URL prefix` to ignore; can repeat to exclude multiple URLs", func(s string) error {
		excludePaths = append(excludePaths, strings.Split(s, ",")...)
		return nil
	})
	dsn := fl.String("sentry-dsn", "", "Sentry DSN `pseudo-URL`")
	shouldArchive := fl.Bool("should-archive", false, "send links to archive.org")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if err := flagext.ParseEnv(fl, "linkrot"); err != nil {
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

	// As of Go 1.13, cookiejar.New always returns nil error
	jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	c := &crawler{
		base.String(),
		*crawlers,
		excludePaths,
		logger,
		&http.Client{
			Jar:     jar,
			Timeout: *timeout,
		},
		chromeUserAgent,
		*shouldArchive,
	}

	c.sentryInit(*dsn)

	return c.run()
}

type crawler struct {
	base         string
	workers      int
	excludePaths []string
	*log.Logger
	*http.Client
	userAgent     string
	shouldArchive bool
}

func (c *crawler) sentryInit(dsn string) {
	sentry.Init(sentry.ClientOptions{
		Dsn: dsn,
	})
}

func (c *crawler) run() error {
	pages, cancelled := c.crawl()
	errs := pages.toURLErrors(c.base)
	c.reportToSentry(errs)
	fmt.Println(errs)
	if c.shouldArchive {
		fmt.Println("archiving links...")
		if err := c.archiveAll(pages); err != nil {
			fmt.Printf("warning: error archiving links %+v\n", err)
		} else {
			fmt.Println("done archiving.")
		}
	}

	var err error
	if cancelled {
		err = ErrCancelled
	} else if len(errs) > 0 {
		err = ErrBadLinks
	}

	return err
}

func (c *crawler) crawl() (crawled crawledPages, cancelled bool) {
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
		// How many fetches we're waiting on
		openFetchs int
	)
	// database of what we've collected
	crawled = newCrawledPages()

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

	return crawled, cancelled
}

func (c *crawler) fetch(url string) fetchResult {
	c.Printf("start fetching %q", url)
	links, ids, err := c.doFetch(url)
	if err == nil {
		c.Printf("done fetching %q", url)
	} else {
		c.Printf("problem fetching %q", url)
	}
	return fetchResult{url, links, ids, err}
}

func (c *crawler) doFetch(pageurl string) (links, ids []string, err error) {
	return c.tryFetch(pageurl, 0)
}

func (c *crawler) tryFetch(pageurl string, try int) (links, ids []string, err error) {
	const (
		maxTries = 3
		tryDelay = 500 * time.Millisecond
	)
	try++
	req, err := http.NewRequest(http.MethodGet, pageurl, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	res, err := c.Client.Do(req)
	if err != nil {
		// Report DNS errors
		if d := new(net.DNSError); errors.As(err, &d) {
			return nil, nil, err
		}
		// Ignore connection errors
		return nil, nil, nil
	}

	defer res.Body.Close()

	if err = statusReject(res,
		http.StatusNotFound,
		http.StatusGone,
	); err != nil {
		return nil, nil, err
	}

	buf := bufio.NewReader(res.Body)
	// http.DetectContentType only uses first 512 bytes
	peek, err := buf.Peek(512)
	if err != nil && err != io.EOF {
		c.Printf("Error initially reading %s body: %v", pageurl, err)
		return nil, nil, nil
	}

	if ct := http.DetectContentType(peek); !strings.HasPrefix(ct, "text/html") && !strings.HasPrefix(ct, "text/xml") {
		c.Printf("Skipping %s, content-type %s", pageurl, ct)
		return nil, nil, nil
	}

	slurp, err := ioutil.ReadAll(buf)
	if err != nil {
		c.Printf("Error reading %s body: %v", pageurl, err)
		return nil, nil, nil
	}

	// If we've been 30X redirected, pageurl will not be response URL
	pageurl = res.Request.URL.String()

	shouldGetLinks := c.shouldGetLinks(pageurl)
	// must be a good URL coz I fetched it
	u, _ := url.Parse(pageurl)
	var allLinks []string
	ids, allLinks, err = getIDsAndLinks(u, slurp, shouldGetLinks)
	if err != nil {
		c.Printf("error parsing HTML: %v", err)
		// TODO: Should we return the error here?
	}

	if shouldGetLinks {
		for _, link := range allLinks {
			c.Printf("url %s links to %s", pageurl, link)

			if !c.isExcluded(link) {
				links = append(links, link)
			}
		}
	}

	return links, ids, nil
}

func (c *crawler) shouldGetLinks(url string) bool {
	return strings.HasPrefix(url, c.base)
}

func (c *crawler) isExcluded(link string) bool {
	if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
		c.Printf("link has excluded protocol: %q", link)
		return true
	}

	for _, prefixPath := range c.excludePaths {
		if strings.HasPrefix(link, prefixPath) {
			return true
		}
	}
	return false
}

func (c *crawler) reportToSentry(errs urlErrors) {
	defer sentry.Flush(10 * time.Second)
	for url, pe := range errs {
		sentry.WithScope(func(scope *sentry.Scope) {
			event := sentry.NewEvent()
			scope.SetFingerprint([]string{url})
			scope.SetTag("URL", url)
			errType := "request error"
			if pe.err == ErrMissingFragment {
				errType = "missing page IDs"
				frags := setToSlice(pe.missingFragments)
				scope.SetExtra("missing page IDs", frags)
			}
			scope.SetTag("failure type", errType)
			scope.SetExtra("affected-pages", pe.refs)
			event.Exception = []sentry.Exception{{
				Type:  url,
				Value: pe.err.Error(),
			}}
			sentry.CaptureEvent(event)
		})
	}
}

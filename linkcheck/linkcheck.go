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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/carlmjohnson/errutil"
	"github.com/carlmjohnson/exitcode"
	"github.com/carlmjohnson/flagext"
	"github.com/carlmjohnson/slackhook"
	"github.com/go-redsync/redsync"
	"github.com/gomodule/redigo/redis"
	"github.com/peterbourgon/ff"
	"golang.org/x/net/publicsuffix"

	"github.com/spotlightpa/linkrot/internal/redisflag"
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

linkcheck [options] <url>

    linkcheck takes a root URL and recurses down through the links it finds
    in the HTML pages, checking for broken links (HTTP status != 200).

    Options may also be specified as env vars prefixed with "LINKROT_".

Options:
`
		fmt.Fprintln(os.Stderr, usage)
		fl.PrintDefaults()
	}

	verbose := fl.Bool("verbose", false, "verbose")
	slack := fl.String("slack-hook-url", "", "send errors to Slack webhook URL")
	crawlers := fl.Int("crawlers", runtime.NumCPU(), "number of concurrent crawlers")
	timeout := fl.Duration("timeout", 10*time.Second, "timeout for requesting a URL")
	var excludePaths []string
	fl.Var((*flagext.Strings)(&excludePaths), "exclude", "URL prefix to ignore; can repeat to exclude multiple URLs")
	getDialer := redisflag.Var(fl, "redis-url", "`URL` connection string for Redis")
	if err := ff.Parse(fl, args, ff.WithEnvVarPrefix("LINKROT")); err != nil {
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
		slackhook.New(*slack, &http.Client{
			Timeout: *timeout,
		}),
		chromeUserAgent,
		nil,
	}

	if dialer := getDialer(); dialer != nil {
		c.rp = &redis.Pool{
			MaxIdle:     3,
			IdleTimeout: 4 * time.Minute,
			Dial:        dialer,
		}
		if err = c.Ping(); err != nil {
			c.Printf("connecting to Redis: %v", err)
			return err
		}
	}

	return c.run()
}

type crawler struct {
	base         string
	workers      int
	excludePaths []string
	*log.Logger
	*http.Client
	sc        *slackhook.Client
	userAgent string
	rp        *redis.Pool
}

func (c *crawler) run() error {
	pages, cancelled := c.crawl()
	errs, connErr := c.filterErrors(pages)
	if connErr != nil {
		c.sc.Post(slackhook.Message{Text: connErr.Error()})
		fmt.Fprintf(os.Stderr, "problem filtering errors: %v\n", connErr)
		return exitcode.Set(connErr, 5)
	}

	if len(errs) > 0 {
		if err := c.sc.Post(errs.toMessage(c.base)); err != nil {
			fmt.Fprintf(os.Stderr, "problem with Slack: %v", err)
		}
	}

	fmt.Println(errs)

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
	req, err := http.NewRequest(http.MethodGet, pageurl, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	res, err := c.Client.Do(req)
	if err != nil {
		return nil, nil, err
	}

	defer res.Body.Close()

	if err = statusCheck(res,
		http.StatusOK,
		// Accepting this because it usually means we hit a paywall
		http.StatusForbidden,
	); err != nil {
		return nil, nil, err
	}

	buf := bufio.NewReader(res.Body)
	// http.DetectContentType only uses first 512 bytes
	peek, err := buf.Peek(512)
	if err != nil && err != io.EOF {
		c.Printf("Error initially reading %s body: %v", pageurl, err)
		return nil, nil, err
	}

	if ct := http.DetectContentType(peek); !strings.HasPrefix(ct, "text/html") && !strings.HasPrefix(ct, "text/xml") {
		c.Printf("Skipping %s, content-type %s", pageurl, ct)
		return nil, nil, nil
	}

	slurp, err := ioutil.ReadAll(buf)
	if err != nil {
		c.Printf("Error reading %s body: %v", pageurl, err)
		return nil, nil, err
	}

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

func (c *crawler) filterErrors(pages crawledPages) (urlErrors, error) {
	errs := pages.toURLErrors(c.base)
	if c.rp == nil {
		c.Println("skipping Redis dedupe")
		return errs, nil
	}

	/*
		If a page has an error and has not had success in > successInt
		and an alert has not been sent in > alertInt
		then send the alert and update the alert time.
	*/
	unlock, err := c.GetLock("filtererrors.lock")
	if err != nil {
		return nil, err
	}
	defer unlock()

	v := struct{ Good, Alert map[string]time.Time }{
		Good:  make(map[string]time.Time),
		Alert: make(map[string]time.Time),
	}
	if err := c.Get("filtererrors.vals", &v); err != nil && err != redis.ErrNil {
		return nil, err
	}

	const (
		successInt = 24 * time.Hour
		alertInt   = 24 * time.Hour
	)
	now := time.Now()

	filteredErrs := make(urlErrors, len(errs))
	for url := range errs {
		if t, ok := v.Good[url]; ok && now.Sub(t) < successInt {
			continue
		}
		if t, ok := v.Alert[url]; ok && now.Sub(t) < alertInt {
			continue
		}
		filteredErrs[url] = errs[url]
		v.Alert[url] = now
	}

	for url := range pages {
		if _, ok := errs[url]; !ok {
			v.Good[url] = now
		}
	}

	if err := c.Set("filtererrors.vals", &v); err != nil {
		return nil, err
	}

	c.Printf("had %d errors, reporting %d", len(errs), len(filteredErrs))
	return filteredErrs, nil
}

// Ping Redis
func (c *crawler) Ping() (err error) {
	c.Println("do Redis PING")
	conn := c.rp.Get()
	defer errutil.Defer(&err, conn.Close)

	_, err = conn.Do("PING")
	return
}

// Get calls GET in Redis and converts values from JSON bytes
func (c *crawler) Get(key string, getv interface{}) (err error) {
	c.Printf("do Redis GET %q", key)
	conn := c.rp.Get()
	defer errutil.Defer(&err, conn.Close)

	getb, err := redis.Bytes(conn.Do("GET", key))
	if err != nil {
		return err
	}
	return json.Unmarshal(getb, getv)
}

// Set converts values to JSON bytes and calls SET in Redis
func (c *crawler) Set(key string, setv interface{}) (err error) {
	c.Printf("do Redis SET %q", key)
	conn := c.rp.Get()
	defer errutil.Defer(&err, conn.Close)

	setb, err := json.Marshal(setv)
	if err != nil {
		return err
	}

	_, err = conn.Do("SET", key, setb)
	return err
}

// GetSet converts values to JSON bytes and calls GETSET in Redis
func (c *crawler) GetSet(key string, getv, setv interface{}) (err error) {
	c.Printf("do Redis GETSET %q", key)
	conn := c.rp.Get()
	defer errutil.Defer(&err, conn.Close)

	setb, err := json.Marshal(setv)
	if err != nil {
		return err
	}
	getb, err := redis.Bytes(conn.Do("GETSET", key, setb))
	if err != nil {
		return err
	}
	return json.Unmarshal(getb, getv)
}

func (c *crawler) GetLock(key string) (unlock func(), err error) {
	c.Printf("get Redis lock %q", key)
	lock := redsync.
		New([]redsync.Pool{c.rp}).
		NewMutex(key)
	if err := lock.Lock(); err != nil {
		return nil, err
	}
	return func() {
		c.Printf("return Redis lock %q", key)
		lock.Unlock()
	}, nil
}

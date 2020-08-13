linkrot
=========

Linkrot takes a root URL and recurses down through the links it finds in the
HTML pages, checking for broken links.

Usage
-----

``` shell
$ linkrot -h
Usage of linkrot:

linkrot [options] <url>

    linkrot takes a root URL and recurses down through the links it finds
    in the HTML pages, checking for broken links (HTTP status != 200).

    Options may also be specified as env vars prefixed with "LINKROT_".

Options:

  -crawlers int
        number of concurrent crawlers (default 8)
  -exclude value
        URL prefix to ignore; can repeat to exclude multiple URLs
  -sentry-dsn pseudo-URL
        Sentry DSN pseudo-URL
  -timeout duration
        timeout for requesting a URL (default 10s)
  -verbose
        verbose

$ linkrot -verbose http://example.com
linkrot 2019/07/23 10:40:54 starting 4 crawlers
linkrot 2019/07/23 10:40:54 Got OK: http://example.com/
linkrot 2019/07/23 10:40:54 url http://example.com/ links to http://www.iana.org/domains/example
linkrot 2019/07/23 10:40:55 Got OK: http://www.iana.org/domains/example
```

Installation
------------

Requires [Go](https://golang.org/) to be installed.

``` shell
$ go get github.com/spotlightpa/linkrot
```

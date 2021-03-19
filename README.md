linkrot
=========

Linkrot takes a root URL and recurses down through the links it finds in the
HTML pages, checking for broken links. Optionally, it can report broken links 
to Sentry.

Usage
-----

```
$ linkrot -h
Usage of linkrot (v0.21.0):

linkrot [options] <url>

    linkrot takes a root URL and recurses down through the links it finds
    in the HTML pages, checking for broken links (HTTP status != 200).

    Options may also be specified as env vars prefixed with "LINKROT_".

Options:

  -crawlers int
        number of concurrent crawlers (default 8)
  -exclude URL prefix
        URL prefix to ignore; can repeat to exclude multiple URLs
  -sentry-dsn pseudo-URL
        Sentry DSN pseudo-URL
  -should-archive
        send links to archive.org
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

```
$ go install github.com/spotlightpa/linkrot@latest
```

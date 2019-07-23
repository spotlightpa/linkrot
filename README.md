linkrot
=========

Linkrot takes a root URL and recurses down through the links it finds in the
HTML pages, checking for broken links.

Usage
-----

``` shell
$ linkrot -h
Usage of linkrot:

linkcheck [options] <url>

    linkcheck takes a root URL and recurses down through the links it finds
    in the HTML pages, checking for broken links (HTTP status != 200).

Options:

  -crawlers int
        number of concurrent crawlers (default 4)
  -exclude string
        comma separated list of URL prefixes to ignore
  -slack-hook-url string
        send errors to Slack webhook URL
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

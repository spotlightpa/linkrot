module github.com/spotlightpa/linkrot

// +heroku goVersion go1.15
// +heroku install ./...

go 1.15

require (
	github.com/carlmjohnson/errutil v0.20.1
	github.com/carlmjohnson/exitcode v0.0.3
	github.com/carlmjohnson/flagext v0.0.11
	github.com/getsentry/sentry-go v0.7.0
	github.com/gomodule/redigo v2.0.0+incompatible
	github.com/peterbourgon/ff v1.6.0
	golang.org/x/net v0.0.0-20191028085509-fe3aa8a45271
)

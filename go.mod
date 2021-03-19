module github.com/spotlightpa/linkrot

// +heroku goVersion go1.16
// +heroku install ./...

go 1.16

require (
	github.com/carlmjohnson/errutil v0.20.1
	github.com/carlmjohnson/exitcode v0.20.2
	github.com/carlmjohnson/flagext v0.21.0
	github.com/getsentry/sentry-go v0.10.0
	golang.org/x/net v0.0.0-20210316092652-d523dce5a7f4
)

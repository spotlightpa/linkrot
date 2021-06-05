module github.com/spotlightpa/linkrot

// +heroku goVersion go1.16
// +heroku install ./...

go 1.16

require (
	github.com/carlmjohnson/errutil v0.20.1
	github.com/carlmjohnson/exitcode v0.20.2
	github.com/carlmjohnson/flagext v0.21.0
	github.com/carlmjohnson/requests v0.21.8
	github.com/getsentry/sentry-go v0.11.0
	golang.org/x/net v0.0.0-20210525063256-abc453219eb5
)

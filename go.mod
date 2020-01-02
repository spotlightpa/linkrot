module github.com/spotlightpa/linkrot

// +heroku goVersion go1.13
// +heroku install ./...

go 1.13

require (
	github.com/carlmjohnson/exitcode v0.0.2
	github.com/carlmjohnson/flagext v0.0.6
	github.com/carlmjohnson/slackhook v0.0.3
	github.com/peterbourgon/ff v1.6.0
	golang.org/x/net v0.0.0-20191028085509-fe3aa8a45271
)

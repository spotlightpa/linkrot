module github.com/spotlightpa/linkrot

// +heroku goVersion go1.13
// +heroku install ./...

go 1.13

require (
	github.com/carlmjohnson/errutil v0.0.9
	github.com/carlmjohnson/exitcode v0.0.3
	github.com/carlmjohnson/flagext v0.0.6
	github.com/carlmjohnson/slackhook v0.0.3
	github.com/go-redsync/redsync v1.3.1
	github.com/gomodule/redigo v2.0.0+incompatible
	github.com/peterbourgon/ff v1.6.0
	github.com/spotlightpa/almanack v0.0.0-20191224154139-bae09f8799db // indirect
	golang.org/x/net v0.0.0-20191028085509-fe3aa8a45271
)

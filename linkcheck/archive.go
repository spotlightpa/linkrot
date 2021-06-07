package linkcheck

import (
	"context"
	"os"
	"os/signal"

	"github.com/carlmjohnson/errutil"
	"github.com/carlmjohnson/requests"
	"golang.org/x/time/rate"
)

func (c *crawler) archiveAll(pages crawledPages) error {
	// queue good URLs
	queue := make([]string, 0, len(pages))
	for u, pi := range pages {
		if pi.err == nil {
			queue = append(queue, u)
		}
	}

	var (
		inflightRequests = 0
		errors           errutil.Slice
		pagesCh          = make(chan string)
		errCh            = make(chan error)
	)

	defer close(pagesCh)
	defer close(errCh)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// See https://archive.org/details/toomanyrequests_20191110
	l := rate.NewLimiter(15.0/60, 15)

	for i := 0; i < c.workers; i++ {
		go func() {
			for page := range pagesCh {
				err := l.Wait(ctx)
				if err == nil {
					err = c.archive(ctx, page)
				}
				errCh <- err
			}
		}()
	}

	for len(queue) > 0 || inflightRequests > 0 {
		var page string
		pagesLoopCh := pagesCh
		if len(queue) > 0 {
			page = queue[0]
		} else {
			pagesLoopCh = nil
		}
		select {
		case pagesLoopCh <- page:
			queue = queue[1:]
			inflightRequests++

		case err := <-errCh:
			inflightRequests--
			errors.Push(err)
			c.Printf("%d pages remaining to archive", len(queue)+inflightRequests)
		}
	}

	return errors.Merge()
}

func (c *crawler) archive(ctx context.Context, page string) error {
	return requests.
		URL("https://web.archive.org").
		Pathf("/save/%s", page).
		Head().
		Client(c.Client).
		Fetch(ctx)
}

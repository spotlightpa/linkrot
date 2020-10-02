package linkcheck

import (
	"io/ioutil"

	"github.com/carlmjohnson/errutil"
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

	for i := 0; i < c.workers; i++ {
		go func() {
			for page := range pagesCh {
				errCh <- c.archive(page)
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
		}
	}

	return errors.Merge()
}

func (c *crawler) archive(page string) error {
	u := "https://web.archive.org/save/" + page
	resp, err := c.Client.Head(u)
	if err != nil {
		return err
	}
	// Drain connection
	_, err = ioutil.ReadAll(resp.Body)
	return err
}

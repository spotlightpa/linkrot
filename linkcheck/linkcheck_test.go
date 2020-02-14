package linkcheck

import (
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	// Silence during test
	log.SetOutput(ioutil.Discard)

	// Special for excluded path test
	excludePaths := []string{"https://example.com/excluded-path"}

	// Test server for our known sites
	ts := httptest.NewServer(http.FileServer(http.Dir("test-fixtures/sample-site")))
	defer ts.Close()

	var testcases = []struct {
		name     string
		base     string
		crawlers int
		errLen   int
		contains string
	}{
		{"basic failure", ts.URL + "/404", 1, 1, "404 Not Found"},
		{"basic success", ts.URL + "/basic-a.html", 1, 0, ""},
		{"more crawlers failure", ts.URL + "/404", 5, 1, "404 Not Found"},
		{"more crawlers success", ts.URL + "/basic-a.html", 5, 0, ""},
		{"circular success", ts.URL + "/circular-a.html", 1, 0, ""},
		{"good external link", ts.URL + "/external-good.html", 1, 0, ""},
		{"bad external link", ts.URL + "/external-bad.html", 1, 1, "https://example.com/404\": 404 Not Found"},
		{"good ID link", ts.URL + "/id-good-a.html", 1, 0, ""},
		{"bad ID link", ts.URL + "/id-bad-a.html", 1, 1, "missing fragment"},
		{"ignore ID link", ts.URL + "/id-ignore-a.html", 1, 0, ""},
		{"excluded path", ts.URL + "/excluded.html", 1, 0, ""},
	}

	for _, test := range testcases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			c := crawler{
				test.base,
				test.crawlers,
				excludePaths,
				log.New(ioutil.Discard, "linkrot", log.LstdFlags),
				http.DefaultClient,
				nil,
				chromeUserAgent,
				nil,
			}

			pages, _ := c.crawl()
			errs := pages.toURLErrors(c.base)
			output := errs.String()

			if len(errs) != test.errLen {
				t.Errorf("Unexpected error length. Got %d; expected %d.",
					len(errs), test.errLen)
			}

			if test.contains == "" && output != "" {
				t.Errorf("Unexpected output. Got %q; expected no output.",
					output)
			} else if !strings.Contains(output, test.contains) {
				t.Errorf("Output missing expected sequence. Got %q; expected to contain %q.",
					output, test.contains)
			}
		})
	}
}

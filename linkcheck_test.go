package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSolver(t *testing.T) {
	ts := httptest.NewServer(http.FileServer(http.Dir("test-fixtures/sample-site")))
	defer ts.Close()

	var testcases = []struct {
		name     string
		base     string
		crawlers int
		exitCode int
	}{
		{"basic failure", ts.URL + "/404", 1, 1},
		{"basic success", ts.URL + "/basic-a.html", 1, 0},
		{"circular success", ts.URL + "/circular-a.html", 1, 0},
		{"good external link", ts.URL + "/external-good.html", 1, 0},
		{"bad external link", ts.URL + "/external-bad.html", 1, 1},
	}

	for _, test := range testcases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if exitCode := run(test.base, test.crawlers); exitCode != test.exitCode {
				t.Errorf("Unexpected exit code. Got %d; expected %d", exitCode, test.exitCode)
			}
		})
	}

}

package linkcheck

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
)

type slackClient struct {
    hookURL string
}

func newSlackClient(hookURL string) *slackClient {
    if hookURL == "" {
        return nil
    }
    return &slackClient{hookURL}
}

func (sc *slackClient) Post(msg interface{}) error {
    blob, err := json.Marshal(msg)
    if err != nil {
        return err
    }
    r := bytes.NewReader(blob)
    rsp, err := http.Post(sc.hookURL, "application/json", r)
    if err != nil {
        return err
    }
    if rsp.StatusCode != http.StatusOK {
        return fmt.Errorf("unexpected status: %q", rsp.Status)
    }
    return nil
}

type field struct {
    Title string `json:"title"`
    Value string `json:"value"`
    Short bool   `json:"short"`
}

type attachment struct {
    Fallback  string  `json:"fallback"`
    Color     string  `json:"color"`
    Title     string  `json:"title"`
    TitleLink string  `json:"title_link"`
    Text      string  `json:"text"`
    TimeStamp int64   `json:"ts"`
    Fields    []field `json:"fields"`
}

type message struct {
    Text        string       `json:"text"`
    Attachments []attachment `json:"attachments"`
}

package netease

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
)

// The days come newest first whatever order NetEase lists them in, a day it
// no longer keeps comes back as an empty list rather than an error, and the
// songs get the privileges the details carry, not the ones the list has.
func TestHistoryRecommend(t *testing.T) {
	old := httpClient
	defer func() { httpClient = old }()
	var detailForm string
	jar, _ := cookiejar.New(nil)
	httpClient = &http.Client{Jar: jar, Transport: browseTransport(func(r *http.Request) (*http.Response, error) {
		var body string
		switch {
		case r.URL.Path == "/api/discovery/recommend/songs/history/recent":
			body = `{"code":200,"data":{"dates":["2026-01-02","2026-01-03"],"description":"近 5 次"}}`
		case r.URL.Path == "/api/discovery/recommend/songs/history/detail":
			r.ParseForm()
			detailForm = r.PostForm.Get("date")
			if detailForm == "2026-01-01" {
				body = `{"code":200,"data":{"songs":[],"noHistoryMessage":"没有这一天的记录"}}`
			} else {
				body = `{"code":200,"data":{"songs":[{"id":7,"name":"song","dt":1000,"fee":1,"privilege":{"id":7,"st":-1,"pl":0},"al":{"id":1,"name":"album","picUrl":"https://example.com/a.jpg"},"ar":[{"id":2,"name":"artist"}]}]}}`
			}
		default:
			t.Fatalf("unexpected request: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	// The details the page's songs get their privileges from: the eapi cannot
	// be reached here, so they answer as NetEase would.
	oldDetails := songDetails
	songDetails = func(ids []int64) ([]Track, error) {
		out := make([]Track, len(ids))
		for i, id := range ids {
			out[i] = Track{ID: id, Privilege: &Privilege{ID: id, St: 0, Pl: 320000}}
		}
		return out, nil
	}
	defer func() { songDetails = oldDetails }()

	days, err := HistoryRecommendDates()
	if err != nil || len(days.Dates) != 2 || days.Dates[0] != "2026-01-03" || days.Note != "近 5 次" {
		t.Fatalf("days %+v: %v", days, err)
	}

	songs, err := HistoryRecommendSongs("2026-01-02")
	if err != nil || len(songs) != 1 {
		t.Fatalf("songs %+v: %v", songs, err)
	}
	if detailForm != "2026-01-02" {
		t.Fatalf("asked for date %q", detailForm)
	}
	if !songs[0].Playable() {
		t.Fatalf("the list's take-down privilege stayed: %+v", songs[0].Privilege)
	}

	none, err := HistoryRecommendSongs("2026-01-01")
	if err != nil || len(none) != 0 {
		t.Fatalf("a day no longer kept: %+v: %v", none, err)
	}
}

// A failing answer is an error, not an empty history.
func TestHistoryRecommendError(t *testing.T) {
	old := httpClient
	defer func() { httpClient = old }()
	jar, _ := cookiejar.New(nil)
	httpClient = &http.Client{Jar: jar, Transport: browseTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":301,"msg":"need login"}`)), Header: make(http.Header)}, nil
	})}
	if _, err := HistoryRecommendDates(); err == nil {
		t.Fatal("a login requirement became an empty history")
	}
}

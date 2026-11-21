package netease

import (
	"net/url"
	"sort"
)

// The daily recommendations NetEase keeps for the listener: the official
// client's 历史日推. It keeps the last few days only — five for a
// subscriber, fewer without one — and says so when asked for more.

// HistoryDays is what NetEase keeps of the listener's daily
// recommendations: the days still there, newest first, and its words
// about them, such as how many it keeps, or why there are none.
type HistoryDays struct {
	Dates []string // YYYY-MM-DD
	Note  string
}

// HistoryRecommendDates returns the days whose daily recommendations NetEase
// still keeps for the listener.
func HistoryRecommendDates() (HistoryDays, error) {
	var res struct {
		apiError
		Data struct {
			Dates       []string `json:"dates"`
			Description string   `json:"description"`
			NoHistory   string   `json:"noHistoryMessage"`
		} `json:"data"`
	}
	path := "/api/discovery/recommend/songs/history/recent"
	if err := post(path, url.Values{}, &res); err != nil {
		return HistoryDays{}, err
	}
	days := res.Data.Dates
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	note := res.Data.Description
	if len(days) == 0 {
		note = res.Data.NoHistory
	}
	return HistoryDays{Dates: days, Note: note}, nil
}

// HistoryRecommendSongs returns the songs NetEase recommended on date,
// YYYY-MM-DD, and none if it no longer keeps that day.
func HistoryRecommendSongs(date string) ([]Track, error) {
	var res struct {
		apiError
		Data struct {
			Songs []Track `json:"songs"`
		} `json:"data"`
	}
	path := "/api/discovery/recommend/songs/history/detail"
	if err := post(path, url.Values{"date": {date}}, &res); err != nil {
		return nil, err
	}
	// These carry privileges, but the web API's deny subscribers songs the
	// eapi streams to them: the details have the right ones, as they do for
	// the day's songs.
	songs := res.Data.Songs
	detailPrivileges(songs)
	return songs, nil
}

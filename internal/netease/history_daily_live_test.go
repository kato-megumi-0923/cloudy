package netease

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
)

// appIdentifier is the installed app's: the session is kept under it, and
// the test binary's own identifier is not one.
const appIdentifier = "io.github.nya-owo.cloud-nya"

// TestHistoryRecommendLive asks NetEase's own endpoints for the dates whose
// daily recommendations it still keeps, and for one of them: what it answers
// decides whether they are reachable as plain /api calls or need weapi.
func TestHistoryRecommendLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to ask NetEase for past daily recommendations")
	}
	secret, err := mygo.Secrets.Service(appIdentifier).Get(sessionSecret)
	if err != nil {
		t.Fatalf("session in the keychain: %v", err)
	}
	httpClient.Jar.SetCookies(siteURL, parseCookie(string(secret)))

	ask := func(path string, form url.Values, os bool) map[string]any {
		req, err := http.NewRequest("POST", apiBase+path, strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", UserAgent)
		req.Header.Set("Referer", apiBase+"/")
		if os {
			req.AddCookie(&http.Cookie{Name: "os", Value: "ios"})
		}
		res, err := httpClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		var out map[string]any
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatalf("%s: %v: %s", path, err, body[:min(300, len(body))])
		}
		return out
	}

	recent := ask("/api/discovery/recommend/songs/history/recent", url.Values{}, true)
	t.Logf("recent: code=%v dates=%v desc=%v no=%v", recent["code"], recent["data"].(map[string]any)["dates"],
		recent["data"].(map[string]any)["description"], recent["data"].(map[string]any)["noHistoryMessage"])

	date := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	detail := ask("/api/discovery/recommend/songs/history/detail", url.Values{"date": {date}}, true)
	data := detail["data"].(map[string]any)
	songs, _ := data["songs"].([]any)
	t.Logf("detail(%s): code=%v songs=%d keys=%v", date, detail["code"], len(songs), keysOf(data))
	if len(songs) > 0 {
		first := songs[0].(map[string]any)
		_, hasPriv := first["privilege"]
		t.Logf("first song keys=%v privilege=%v fee=%v st=%v", keysOf(first), hasPriv, first["fee"], first["st"])
		if al, ok := first["al"].(map[string]any); ok {
			t.Logf("album keys=%v", keysOf(al))
		}
	}

	noOS := ask("/api/discovery/recommend/songs/history/detail", url.Values{"date": {date}}, false)
	noData := noOS["data"].(map[string]any)
	noSongs, _ := noData["songs"].([]any)
	t.Logf("detail(%s) without os cookie: code=%v songs=%d", date, noOS["code"], len(noSongs))

	old := ask("/api/discovery/recommend/songs/history/detail", url.Values{"date": {"2026-10-01"}}, true)
	oldData := old["data"].(map[string]any)
	oldSongs, _ := oldData["songs"].([]any)
	t.Logf("detail(2026-10-01): code=%v songs=%d no=%v desc=%v", old["code"], len(oldSongs),
		oldData["noHistoryMessage"], oldData["description"])
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

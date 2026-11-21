package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/download"
	"netease-music-mygo/internal/netease"
)

// The listener may give their own playlists and the songs whose files are
// here a cover of their own, in Cloudy alone (a viewer, 1.3.2: the covers
// they had chosen for their music were NetEase's once imported). Nothing
// is uploaded and no file is written to: the picture is copied beside the
// settings, and what it stands for is noted in own-covers.json.

// ownCover is a cover the listener chose: the copy of their picture, and
// for a song, the cover it had before, which it gets back.
type ownCover struct {
	Path string `json:"path"`
	Orig string `json:"orig,omitempty"`
}

// ownCovers are the covers chosen, by ownPlaylistKey or ownSongKey.
type ownCovers struct {
	loaded bool
	m      map[string]ownCover
}

func ownPlaylistKey(id int64) string { return fmt.Sprintf("playlist:%d", id) }
func ownSongKey(path string) string  { return "file:" + downloadPathKey(path) }

// ownCoversDir is where the copies are kept, beside the settings: none for
// settings never loaded, as in tests.
func (a *app) ownCoversDir() string {
	if a.settings.path == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(a.settings.path), "own-covers")
}

func (a *app) ownCoversFile() string {
	if dir := a.ownCoversDir(); dir != "" {
		return dir + ".json"
	}
	return ""
}

// ownCoverMap reads own-covers.json the first time it is asked.
func (a *app) ownCoverMap() map[string]ownCover {
	oc := &a.ownCovers
	if !oc.loaded {
		oc.loaded = true
		oc.m = map[string]ownCover{}
		if path := a.ownCoversFile(); path != "" {
			if b, err := os.ReadFile(path); err == nil {
				json.Unmarshal(b, &oc.m)
			}
		}
		if oc.m == nil {
			oc.m = map[string]ownCover{}
		}
	}
	return oc.m
}

func (a *app) saveOwnCovers() {
	path := a.ownCoversFile()
	if path == "" {
		return
	}
	b, err := json.MarshalIndent(a.ownCoverMap(), "", "  ")
	if err == nil {
		os.MkdirAll(filepath.Dir(path), 0o700)
		writeFileAtomic(path, b, 0o600)
	}
}

// useOwnCovers has playlists show the covers chosen for them.
func (a *app) useOwnCovers() {
	netease.PlaylistCover = func(id int64) string {
		if oc, ok := a.ownCoverMap()[ownPlaylistKey(id)]; ok {
			return oc.Path
		}
		return ""
	}
}

// applyOwnCovers gives the songs of items the covers chosen for them.
func (a *app) applyOwnCovers(items []download.Item) {
	m := a.ownCoverMap()
	if len(m) == 0 {
		return
	}
	for i := range items {
		if oc, ok := m[ownSongKey(items[i].Path)]; ok {
			items[i].Track.Album.PicURL = oc.Path
		}
	}
}

// hasOwnCover reports whether the listener chose the cover under key.
func (a *app) hasOwnCover(key string) bool {
	_, ok := a.ownCoverMap()[key]
	return ok
}

// ownCoverImage reads the picture at path, as a cover may be: a PNG or a
// JPEG, not too large.
func ownCoverImage(path string) ([]byte, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", err
	}
	if info.Size() > 20<<20 {
		return nil, "", fmt.Errorf("图片太大了（超过 20 MB）")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	_, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, "", fmt.Errorf("无法读取这张图片，请选择 PNG 或 JPG")
	}
	ext := ".jpg"
	if format == "png" {
		ext = ".png"
	}
	return b, ext, nil
}

// chooseOwnCoverFile asks for the picture; tests replace it.
var chooseOwnCoverFile = func(win *mygo.Window) (string, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: "选择封面图片",
		Filters: []mygo.FileFilter{{Name: "图片", Extensions: []string{"png", "jpg", "jpeg"}}}})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}

// pickOwnCover asks for a picture and makes it the cover under key: a
// playlist's, or the song's whose file is at path.
func (a *app) pickOwnCover(key, songPath string) {
	win := a.win
	go func() {
		path, err := chooseOwnCoverFile(win)
		if err != nil || path == "" {
			return
		}
		b, ext, err := ownCoverImage(path)
		a.post(func() {
			if err != nil {
				a.toast = err.Error()
				return
			}
			if err := a.setOwnCover(key, songPath, b, ext); err != nil {
				a.toast = "无法保存封面：" + err.Error()
				return
			}
			a.toast = "已更换封面（只在 Cloudy 里显示）"
		})
	}()
}

// setOwnCover keeps picture b as the cover under key.
func (a *app) setOwnCover(key, songPath string, b []byte, ext string) error {
	dir := a.ownCoversDir()
	if dir == "" {
		return fmt.Errorf("没有数据文件夹")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	h := fnv.New64a()
	h.Write([]byte(key))
	// A new name each time: the cover cache keeps pictures by their path.
	path := filepath.Join(dir, fmt.Sprintf("%x-%d%s", h.Sum64(), time.Now().UnixNano(), ext))
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	m := a.ownCoverMap()
	was, had := m[key]
	oc := ownCover{Path: path}
	if had {
		os.Remove(was.Path)
		oc.Orig = was.Orig
	} else if songPath != "" {
		oc.Orig = a.songCoverNow(songPath)
	}
	m[key] = oc
	a.saveOwnCovers()
	a.ownCoversChanged(songPath)
	return nil
}

// resetOwnCover gives back the cover under key that NetEase or the file
// had.
func (a *app) resetOwnCover(key, songPath string) {
	m := a.ownCoverMap()
	oc, ok := m[key]
	if !ok {
		return
	}
	delete(m, key)
	os.Remove(oc.Path)
	a.saveOwnCovers()
	if songPath != "" {
		for _, items := range [][]download.Item{a.dl.items, a.lib.items} {
			for i := range items {
				if downloadPathKey(items[i].Path) == downloadPathKey(songPath) {
					items[i].Track.Album.PicURL = oc.Orig
				}
			}
		}
	}
	a.ownCoversChanged(songPath)
	a.toast = "已恢复原封面"
}

// songCoverNow is the cover the song at path shows, before the listener's.
func (a *app) songCoverNow(path string) string {
	for _, items := range [][]download.Item{a.dl.items, a.lib.items} {
		for _, it := range items {
			if downloadPathKey(it.Path) == downloadPathKey(path) {
				return it.Track.Album.PicURL
			}
		}
	}
	return ""
}

// ownCoversChanged shows the covers as they now are, and keeps the indexes
// of the songs, whose covers are in them.
func (a *app) ownCoversChanged(songPath string) {
	if songPath != "" {
		a.applyOwnCovers(a.dl.items)
		a.noteDownloads()
		a.saveDownloads()
		if len(a.lib.items) > 0 {
			a.setLibrary(a.lib.items)
			a.libraryStore().Save(a.lib.items)
		}
	}
	a.redraw()
}

// ownCoverActions are the menu's rows to change the cover under key, and to
// have the old one back once changed.
func (a *app) ownCoverActions(key, songPath string) []menuAction {
	acts := []menuAction{{label: "更换封面…", icon: iconImage, do: func() { a.pickOwnCover(key, songPath) }}}
	if a.hasOwnCover(key) {
		acts = append(acts, menuAction{label: "恢复原封面", icon: iconUndo, do: func() { a.resetOwnCover(key, songPath) }})
	}
	return acts
}

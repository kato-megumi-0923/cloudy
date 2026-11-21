package app

import "time"

// volumeKeepAfter is how long the volume must stay put before it is kept.
const volumeKeepAfter = time.Second

// keepVolume keeps the volume in the settings as it moves, and writes them
// once it has stayed put a moment: kept only as the app quit, it was lost
// whenever Windows ended the app with the session, and came back at 80%.
func (a *app) keepVolume() {
	if a.settings.Volume == a.volume {
		return
	}
	a.settings.Volume = a.volume
	if a.volumeSave != nil {
		a.volumeSave.Stop()
	}
	a.volumeSave = time.AfterFunc(volumeKeepAfter, func() {
		a.post(func() { a.settings.save() })
	})
}

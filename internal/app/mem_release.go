package app

import (
	"runtime/debug"
	"time"
)

// releaseDelay is how long after a heavy view goes that releaseSoon gives
// its memory back: past the frames that let go of it, and late enough
// that turning it on and off again in quick turns pays once.
const releaseDelay = 1500 * time.Millisecond

// releaseSoon gives back the memory of a view just gone (Now Playing
// closed, the desk lyrics or the mini player shut), once releaseDelay
// passes with no other going. Left to itself, Go collects an idle heap
// only every two minutes, and the renderer frees a canvas's texture only
// once the canvas is collected: what the view took would wait that long,
// and longer for the memory Go keeps after collecting. FreeOSMemory
// collects at once and hands the free pages back. Call it on the main
// thread.
func (a *app) releaseSoon() {
	if a.releaseT != nil {
		a.releaseT.Reset(releaseDelay)
		return
	}
	a.releaseT = time.AfterFunc(releaseDelay, debug.FreeOSMemory)
}

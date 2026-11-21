//go:build !windows

package player

// openOther opens audio in format, which the player's own decoders do not
// read: none, off Windows.
func openOther(dl *download, durationMs int64, format string) (stream, error) {
	return nil, &UnsupportedError{Format: format}
}

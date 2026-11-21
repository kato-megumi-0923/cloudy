package player

// openOther opens audio in format, which the player's own decoders do not
// read: Media Foundation decodes AAC, in an M4A (with ALAC too) or bare,
// and WMA.
func openOther(dl *download, durationMs int64, format string) (stream, error) {
	switch format {
	case "AAC (M4A)", "AAC", "WMA":
		return openMFStream(dl, durationMs, format)
	}
	return nil, &UnsupportedError{Format: format}
}

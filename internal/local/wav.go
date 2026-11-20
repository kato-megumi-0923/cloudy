package local

import (
	"encoding/binary"
	"strings"
)

func readWAVInfo(data []byte, song *Song) {
	if len(data) < 4 || string(data[:4]) != "INFO" {
		return
	}
	for offset := int64(4); offset+8 <= int64(len(data)); {
		size := int64(binary.LittleEndian.Uint32(data[offset+4:]))
		end := offset + 8 + size
		if end > int64(len(data)) {
			return
		}
		value := strings.TrimRight(string(data[offset+8:end]), "\x00")
		switch string(data[offset : offset+4]) {
		case "INAM":
			if song.Title == "" {
				song.Title = value
			}
		case "IART":
			if song.Artist == "" {
				song.Artist = value
			}
		case "IPRD":
			if song.Album == "" {
				song.Album = value
			}
		case "ITRK":
			if song.Track == 0 {
				song.Track = number(value)
			}
		}
		offset = end + size%2
	}
}

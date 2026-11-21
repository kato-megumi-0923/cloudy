package lyrics

import (
	"fmt"
	"strings"
	"time"
)

// ToLRC writes lines as an LRC file: a line's time in square brackets, its
// words' times in angle brackets where the words have times of their own
// (enhanced LRC), and its translation on the line below, carrying the
// line's time too so that a reader pairs the two again.
//
// Words only spread evenly along a line have no times of their own, and
// are written as plain lines: a file says what is sung word by word only
// where that is known.
func ToLRC(lines []Line) string {
	var out strings.Builder
	for _, l := range lines {
		text := strings.TrimSpace(l.Text)
		trans := strings.TrimSpace(l.Translation)
		if text == "" && trans == "" {
			continue
		}
		stamp := "[" + lrcClock(l.At) + "]"
		if text == "" {
			// A translation without its line: the line's time is all it has.
			out.WriteString(stamp + trans + "\r\n")
			continue
		}
		words, timed := lrcWords(l)
		if timed {
			out.WriteString(stamp)
			for _, w := range words {
				out.WriteString("<" + lrcClock(w.Start) + ">" + w.Text)
			}
			if end := l.End; end > 0 && end >= words[len(words)-1].End {
				// The last marker, with nothing after it, ends the line:
				// without it a reader would run the last word on.
				out.WriteString("<" + lrcClock(end) + ">")
			}
			out.WriteString("\r\n")
		} else {
			out.WriteString(stamp + text + "\r\n")
		}
		if trans == "" {
			continue
		}
		if timed {
			out.WriteString(stamp + "<" + lrcClock(l.At) + ">" + trans)
			if l.End > l.At {
				out.WriteString("<" + lrcClock(l.End) + ">")
			}
			out.WriteString("\r\n")
		} else {
			out.WriteString(stamp + trans + "\r\n")
		}
	}
	return out.String()
}

// HasWordTimes reports whether text times its words, as enhanced LRC does:
// the lyrics that follow a song word by word, rather than line by line.
func HasWordTimes(text string) bool {
	return enhancedLRCTime.MatchString(text)
}

// lrcWords are the words of l worth timing one by one: only those of a line
// whose words carry their own times, and only when the line has any.
func lrcWords(l Line) ([]Word, bool) {
	if !l.Timed {
		return nil, false
	}
	words := make([]Word, 0, len(l.Words))
	for _, w := range l.Words {
		if w.Text != "" {
			words = append(words, w)
		}
	}
	return words, len(words) > 0
}

// lrcClock is a time as LRC writes it: minutes, seconds, milliseconds.
func lrcClock(at time.Duration) string {
	ms := max(0, at.Milliseconds())
	return fmt.Sprintf("%02d:%02d.%03d", ms/60000, ms/1000%60, ms%1000)
}

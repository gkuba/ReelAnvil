package pipeline

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Cue is one burned caption: a source-time span and its text.
type Cue struct {
	From, To int64
	Text     string
}

// WordsPerCue is how many words a word-by-word caption shows at once; the
// sentence style uses CaptionWords.
const WordsPerCue = 3

// WordCues splits each transcript line that overlaps clip into cues of at
// most per words. The transcript only has times for whole lines, so words are
// timed by their length within their line: close for short lines, not exact.
func WordCues(segs []Segment, clip Span, per int) []Cue {
	if per < 1 {
		per = 1
	}
	var cues []Cue
	for _, s := range segs {
		if s.EndMS <= clip.StartMS || s.StartMS >= clip.EndMS {
			continue
		}
		words := strings.Fields(s.Text)
		total := 0
		for _, w := range words {
			total += utf8.RuneCountInString(w) + 1
		}
		if total == 0 {
			continue
		}
		span := s.EndMS - s.StartMS
		at, used := s.StartMS, 0
		for i := 0; i < len(words); i += per {
			j := min(i+per, len(words))
			for _, w := range words[i:j] {
				used += utf8.RuneCountInString(w) + 1
			}
			to := s.StartMS + span*int64(used)/int64(total)
			if j == len(words) {
				to = s.EndMS
			}
			from, end := max(at, clip.StartMS), min(to, clip.EndMS)
			at = to
			if end-from < 120 {
				continue
			}
			cues = append(cues, Cue{from, end, strings.Join(words[i:j], " ")})
		}
	}
	return cues
}

var fillerLine = regexp.MustCompile(`(?i)^(?:(?:u+h+|u+m+|e+r+m*|a+h+|h+m+|m+h*m+)[\s.,!?…-]*)+$`)

// FillerSpans returns the time of transcript lines that hold only filler
// sounds ("Um.", "Uh, uh."). Fillers inside a sentence cannot be cut cleanly
// because the transcript has no word times.
func FillerSpans(segs []Segment) []Span {
	var out []Span
	for _, s := range segs {
		if t := strings.TrimSpace(s.Text); t != "" && fillerLine.MatchString(t) {
			out = append(out, Span{s.StartMS, s.EndMS})
		}
	}
	return out
}

const (
	// MinCutMS is the shortest silence that is cut; shorter pauses are speech rhythm.
	MinCutMS = 700
	// KeepPadMS stays on each side of a cut silence so speech is not clipped.
	KeepPadMS = 200
	minKeepMS = 200
)

// KeepSpans returns the parts of clip to keep, in source time, once long
// silences (at least MinCutMS, leaving KeepPadMS each side) and filler-only
// lines are removed. With nothing to cut it returns just clip.
func KeepSpans(clip Span, silences, fillers []Span) []Span {
	var cuts []Span
	for _, s := range silences {
		if s.EndMS-s.StartMS < MinCutMS || s.EndMS <= clip.StartMS || s.StartMS >= clip.EndMS {
			continue
		}
		a, b := s.StartMS+KeepPadMS, s.EndMS-KeepPadMS
		if s.StartMS <= clip.StartMS {
			a = clip.StartMS
		}
		if s.EndMS >= clip.EndMS {
			b = clip.EndMS
		}
		cuts = append(cuts, Span{max(a, clip.StartMS), min(b, clip.EndMS)})
	}
	for _, f := range fillers {
		if f.EndMS > clip.StartMS && f.StartMS < clip.EndMS {
			cuts = append(cuts, Span{max(f.StartMS, clip.StartMS), min(f.EndMS, clip.EndMS)})
		}
	}
	// Sort and merge the cuts, then keep what is between them.
	for i := 1; i < len(cuts); i++ {
		for j := i; j > 0 && cuts[j].StartMS < cuts[j-1].StartMS; j-- {
			cuts[j], cuts[j-1] = cuts[j-1], cuts[j]
		}
	}
	keep := []Span{}
	at := clip.StartMS
	for _, c := range cuts {
		if c.EndMS <= c.StartMS || c.EndMS <= at {
			continue
		}
		if c.StartMS-at >= minKeepMS {
			keep = append(keep, Span{at, c.StartMS})
		}
		at = max(at, c.EndMS)
	}
	if clip.EndMS-at >= minKeepMS {
		keep = append(keep, Span{at, clip.EndMS})
	}
	if len(keep) == 0 {
		return []Span{clip}
	}
	return keep
}

// Total is the combined length of spans.
func Total(spans []Span) int64 {
	var n int64
	for _, s := range spans {
		n += s.EndMS - s.StartMS
	}
	return n
}

// Remap converts a source time to its time in the clip made from keep. A
// time inside a removed part lands on the next kept part.
func Remap(keep []Span, t int64) int64 {
	var out int64
	for _, k := range keep {
		if t < k.StartMS {
			return out
		}
		if t < k.EndMS {
			return out + t - k.StartMS
		}
		out += k.EndMS - k.StartMS
	}
	return out
}

func remapCues(cues []Cue, keep []Span) []Cue {
	var out []Cue
	for _, c := range cues {
		from, to := Remap(keep, c.From), Remap(keep, c.To)
		if to-from >= 120 {
			out = append(out, Cue{from, to, c.Text})
		}
	}
	return out
}

// CuesSRT writes cues as .srt in the time of the clip made from keep.
func CuesSRT(cues []Cue, keep []Span) string {
	var sb strings.Builder
	for i, c := range remapCues(cues, keep) {
		fmt.Fprintf(&sb, "%d\n%s --> %s\n%s\n\n", i+1, srtTime(c.From), srtTime(c.To), c.Text)
	}
	return sb.String()
}

// TitleMS is how long the title card shows at the start of a clip.
const TitleMS = 2500

func assTime(ms int64) string {
	return fmt.Sprintf("%d:%02d:%02d.%02d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000/10)
}

func assText(s string) string {
	r := strings.NewReplacer("{", "(", "}", ")", "\\", " ", "\n", " ", "\r", " ")
	return strings.TrimSpace(r.Replace(s))
}

// ASS writes the burned captions for a 1080x1920 clip made from keep: the
// cues, and (when title is not empty) a title card over the first TitleMS.
func ASS(cues []Cue, title string, keep []Span) string {
	var sb strings.Builder
	sb.WriteString(`[Script Info]
ScriptType: v4.00+
PlayResX: 1080
PlayResY: 1920
WrapStyle: 0
ScaledBorderAndShadow: yes

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Default,Arial,86,&H00FFFFFF,&H00FFFFFF,&H00000000,&H00000000,1,0,0,0,100,100,0,0,1,7,0,2,60,60,470,1
Style: Title,Arial,64,&H00FFFFFF,&H00FFFFFF,&H00000000,&H99000000,1,0,0,0,100,100,0,0,3,22,0,8,90,90,230,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
`)
	if t := assText(title); t != "" {
		fmt.Fprintf(&sb, "Dialogue: 1,%s,%s,Title,,0,0,0,,%s\n", assTime(0), assTime(min(TitleMS, Total(keep))), t)
	}
	for _, c := range remapCues(cues, keep) {
		fmt.Fprintf(&sb, "Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n", assTime(c.From), assTime(c.To), assText(c.Text))
	}
	return sb.String()
}

// Package pipeline holds the pure parts of the media pipeline: cutting audio
// into transcription segments, building and checking model prompts, choosing
// clips and writing captions. It does no I/O.
package pipeline

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Span is a stretch of source time in milliseconds.
type Span struct{ StartMS, EndMS int64 }

// Segment is transcribed speech in source time.
type Segment struct {
	StartMS, EndMS int64
	Text           string
}

// Segmentation limits for transcription.
const (
	SegMinMS    = 3000
	SegTargetMS = 8000
	SegMaxMS    = 12000
)

// Cut splits [0, durationMS) into spans for transcription, cutting at the
// middle of detected silences where possible so words are not split. Spans
// are SegMinMS..SegMaxMS long, except a shorter last one; with no usable
// silence the cut is made at SegMaxMS.
func Cut(durationMS int64, silences []Span) []Span {
	var points []int64
	for _, s := range silences {
		if s.EndMS > s.StartMS {
			points = append(points, (s.StartMS+s.EndMS)/2)
		}
	}
	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })
	var out []Span
	start := int64(0)
	for durationMS-start > SegMaxMS {
		best, bestDist := int64(-1), int64(-1)
		for _, p := range points {
			l := p - start
			if l < SegMinMS {
				continue
			}
			if l > SegMaxMS {
				break
			}
			d := l - SegTargetMS
			if d < 0 {
				d = -d
			}
			if best < 0 || d < bestDist {
				best, bestDist = p, d
			}
		}
		if best < 0 {
			best = start + SegMaxMS
		}
		out = append(out, Span{start, best})
		start = best
	}
	if durationMS > start {
		out = append(out, Span{start, durationMS})
	}
	return out
}

// Brief is the campaign context given to both models.
type Brief struct{ Title, Requirements string }

func (b Brief) text() string {
	if b.Title == "" && b.Requirements == "" {
		return "No campaign. Pick moments that work as stand-alone short videos."
	}
	return fmt.Sprintf("Campaign: %s\nRequirements: %s", b.Title, oneLine(b.Requirements, 1500))
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > max {
		r := []rune(s)
		s = string(r[:max]) + "…"
	}
	return s
}

// Clock formats milliseconds as m:ss.
func Clock(ms int64) string {
	s := ms / 1000
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// Windows splits segment indexes into consecutive ranges whose numbered
// text stays under maxChars, so a long transcript fits the model context.
func Windows(segs []Segment, maxChars int) [][2]int {
	var out [][2]int
	start, size := 0, 0
	for i, s := range segs {
		n := len(s.Text) + 24
		if size > 0 && size+n > maxChars {
			out = append(out, [2]int{start, i - 1})
			start, size = i, 0
		}
		size += n
	}
	if len(segs) > 0 {
		out = append(out, [2]int{start, len(segs) - 1})
	}
	return out
}

// NotAClip names talk that never makes a clip on its own. Both the hunter and
// the reviewer get it.
const NotAClip = `Greetings, introductions, "are you ready" or sound and voice checks, thanks and sign-offs, and scheduling or housekeeping talk are not clips. A clip needs real substance: an answer, a story, a fact, an opinion or a reaction.`

// HunterPrompt asks for clip proposals inside segs[from..to].
func HunterPrompt(b Brief, segs []Segment, from, to, want, minS, maxS int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, `You pick short vertical video clips from a transcript.
%s

Find up to %d moments that work on their own: a strong opening line (hook), a clear point, and an ending that pays it off. Each clip must be %d to %d seconds long. Clips must not overlap. End a clip only where the speakers have finished the whole answer or exchange, never while someone is still talking.
%s

Transcript lines are numbered: [number] start-end text.
`, b.text(), want, minS, maxS, NotAClip)
	for i := from; i <= to; i++ {
		fmt.Fprintf(&sb, "[%d] %s-%s %s\n", i, Clock(segs[i].StartMS), Clock(segs[i].EndMS), segs[i].Text)
	}
	sb.WriteString(`
Reply with JSON only, in this form:
{"clips":[{"start":<first line number>,"end":<last line number>,"title":"<short title>","hook":"<the opening line>","payoff":"<how it ends>","why":"<why it works>","strength":<1-10>}]}`)
	return sb.String()
}

// Pick is a validated clip proposal as segment indexes.
type Pick struct {
	Start, End          int
	Title, Hook, Payoff string
	Why                 string
	Strength            int
}

// Span returns the pick's source time.
func (p Pick) Span(segs []Segment) Span { return Span{segs[p.Start].StartMS, segs[p.End].EndMS} }

type rawPick struct {
	Start    any    `json:"start"`
	End      any    `json:"end"`
	Title    string `json:"title"`
	Hook     string `json:"hook"`
	Payoff   string `json:"payoff"`
	Why      string `json:"why"`
	Strength any    `json:"strength"`
}

// ParseHunt checks a hunter reply against the window and length limits. It
// returns the valid picks and a list of problems to send back on a retry.
func ParseHunt(raw string, segs []Segment, from, to, minS, maxS int) ([]Pick, []string) {
	var reply struct {
		Clips []rawPick `json:"clips"`
	}
	if err := json.Unmarshal([]byte(extractJSON(raw)), &reply); err != nil {
		return nil, []string{"The reply was not the JSON form asked for."}
	}
	var picks []Pick
	var problems []string
	for i, r := range reply.Clips {
		start, ok1 := toInt(r.Start)
		end, ok2 := toInt(r.End)
		if !ok1 || !ok2 {
			problems = append(problems, fmt.Sprintf("Clip %d: start and end must be line numbers.", i+1))
			continue
		}
		if start < int64(from) || end > int64(to) || start > end {
			problems = append(problems, fmt.Sprintf("Clip %d: lines %d-%d are outside %d-%d.", i+1, start, end, from, to))
			continue
		}
		p := Pick{Start: int(start), End: int(end), Title: oneLine(r.Title, 120), Hook: oneLine(r.Hook, 300),
			Payoff: oneLine(r.Payoff, 300), Why: oneLine(r.Why, 500)}
		if s, ok := toInt(r.Strength); ok && s >= 1 && s <= 10 {
			p.Strength = int(s)
		} else {
			p.Strength = 5
		}
		p.End = fitEnd(segs, p.Start, p.End, to, minS, maxS)
		sp := p.Span(segs)
		length := (sp.EndMS - sp.StartMS) / 1000
		if length < int64(minS) || length > int64(maxS) {
			problems = append(problems, fmt.Sprintf("Clip %d: lines %d-%d last %d seconds, not %d-%d.", i+1, start, end, length, minS, maxS))
			continue
		}
		p.End = finishSentence(segs, p.Start, p.End, to, maxS)
		sp = p.Span(segs)
		overlap := false
		for _, q := range picks {
			if p.Start <= q.End && q.Start <= p.End {
				overlap = true
			}
		}
		if overlap {
			problems = append(problems, fmt.Sprintf("Clip %d overlaps another clip.", i+1))
			continue
		}
		if p.Title == "" {
			p.Title = fmt.Sprintf("Clip at %s", Clock(sp.StartMS))
		}
		picks = append(picks, p)
	}
	if len(picks) == 0 && len(problems) == 0 {
		problems = append(problems, "The reply had no clips.")
	}
	return picks, problems
}

// fitEnd keeps the chosen opening line and moves the last line forward or
// back, within the window, until the clip lasts minS to maxS seconds when it
// can. Small models often pick a good start with the wrong length.
func fitEnd(segs []Segment, start, end, to, minS, maxS int) int {
	secs := func(e int) int64 { return (segs[e].EndMS - segs[start].StartMS) / 1000 }
	for end < to && secs(end) < int64(minS) {
		end++
	}
	for end > start && secs(end) > int64(maxS) {
		end--
	}
	return end
}

// FinishCapS is how long a clip may grow to finish what is being said, when
// the job's own maximum is shorter.
const FinishCapS = 120

// finishSentence moves the end forward, within the window, to the first line
// that finishes a sentence. Lines are cut at pauses, not sentences, and a
// clip must not stop while someone is still talking, so this may run past
// maxS up to FinishCapS; it never cuts a clip shorter.
func finishSentence(segs []Segment, start, end, to, maxS int) int {
	limit := int64(max(maxS, FinishCapS)) * 1000
	for e := end; e <= to && segs[e].EndMS-segs[start].StartMS <= limit; e++ {
		if endsSentence(segs[e].Text) {
			return e
		}
	}
	return end
}

// endsSentence reports whether a transcript line finishes a sentence.
func endsSentence(s string) bool {
	s = strings.TrimRight(strings.TrimSpace(s), `"')”’`)
	if strings.HasSuffix(s, "...") || strings.HasSuffix(s, "…") {
		return false
	}
	return strings.HasSuffix(s, ".") || strings.HasSuffix(s, "!") || strings.HasSuffix(s, "?")
}

// toInt reads a whole number the model may have sent as a number or string.
func toInt(v any) (int64, bool) {
	switch x := v.(type) {
	case float64:
		return int64(x), x == float64(int64(x))
	case string:
		var n int64
		_, err := fmt.Sscan(strings.TrimSpace(x), &n)
		return n, err == nil
	}
	return 0, false
}

// RetryPrompt adds the problems of the previous reply to the prompt.
func RetryPrompt(prompt string, problems []string) string {
	return prompt + "\n\nYour previous reply had these problems; fix them:\n- " + strings.Join(problems, "\n- ")
}

// extractJSON trims any text around the outermost JSON object.
func extractJSON(s string) string {
	i, j := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if i < 0 || j < i {
		return s
	}
	return s[i : j+1]
}

// Best returns up to n non-overlapping picks, strongest first, then in
// source order.
func Best(picks []Pick, n int) []Pick {
	sorted := append([]Pick(nil), picks...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Strength > sorted[j].Strength })
	var out []Pick
	for _, p := range sorted {
		ok := true
		for _, q := range out {
			if p.Start <= q.End && q.Start <= p.End {
				ok = false
			}
		}
		if ok && len(out) < n {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

// Text joins segs[from..to].
func Text(segs []Segment, from, to int) string {
	var parts []string
	for i := from; i <= to && i < len(segs); i++ {
		if i >= 0 {
			parts = append(parts, segs[i].Text)
		}
	}
	return strings.Join(parts, " ")
}

// ReviewContext is how many segments around a candidate the reviewer sees.
const ReviewContext = 2

// ReviewPrompt asks an independent reviewer to judge one candidate. It gets
// the clip text, the text around it and the brief, and nothing from the
// hunter's reasoning.
func ReviewPrompt(b Brief, segs []Segment, start, end int) string {
	before := Text(segs, start-ReviewContext, start-1)
	after := Text(segs, end+1, end+ReviewContext)
	sp := Span{segs[start].StartMS, segs[end].EndMS}
	return fmt.Sprintf(`You review a short vertical video clip cut from a longer video, before a person sees it.
%s

Accept the clip only if it makes sense on its own without the surrounding video, starts with something that holds attention, ends at a natural point, and fits the campaign requirements. Reject it if it stops before the speaker has finished what they were saying. Reject it otherwise.
%s

Before the clip: %s
Clip (%d seconds): %s
After the clip: %s

Reply with JSON only: {"verdict":"accept" or "reject","reason":"<one sentence>"}`,
		b.text(), NotAClip, orNone(before), (sp.EndMS-sp.StartMS)/1000, Text(segs, start, end), orNone(after))
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(nothing)"
	}
	return s
}

// ParseReview reads a reviewer reply.
func ParseReview(raw string) (verdict, reason string, err error) {
	var r struct {
		Verdict string `json:"verdict"`
		Reason  string `json:"reason"`
	}
	if err = json.Unmarshal([]byte(extractJSON(raw)), &r); err != nil {
		return "", "", fmt.Errorf("the reply was not JSON")
	}
	v := strings.ToLower(strings.TrimSpace(r.Verdict))
	if v != "accept" && v != "reject" {
		return "", "", fmt.Errorf("unknown verdict %q", r.Verdict)
	}
	return v, oneLine(r.Reason, 500), nil
}

// CaptionWords is the most words on one caption line.
const CaptionWords = 6

// SRT writes captions for the clip span, in clip-relative time. Each segment's
// text is split into short lines spread over the segment by text length.
func SRT(segs []Segment, clip Span) string {
	var sb strings.Builder
	n := 0
	for _, s := range segs {
		if s.EndMS <= clip.StartMS || s.StartMS >= clip.EndMS {
			continue
		}
		words := strings.Fields(s.Text)
		if len(words) == 0 {
			continue
		}
		var lines []string
		for i := 0; i < len(words); i += CaptionWords {
			j := i + CaptionWords
			if j > len(words) {
				j = len(words)
			}
			lines = append(lines, strings.Join(words[i:j], " "))
		}
		total := 0
		for _, l := range lines {
			total += utf8.RuneCountInString(l)
		}
		span := s.EndMS - s.StartMS
		at := s.StartMS
		for k, l := range lines {
			d := span * int64(utf8.RuneCountInString(l)) / int64(total)
			from, to := at, at+d
			if k == len(lines)-1 {
				to = s.EndMS
			}
			at = to
			if to <= clip.StartMS || from >= clip.EndMS {
				continue
			}
			from, to = max(from, clip.StartMS)-clip.StartMS, min(to, clip.EndMS)-clip.StartMS
			if to-from < 200 {
				continue
			}
			n++
			fmt.Fprintf(&sb, "%d\n%s --> %s\n%s\n\n", n, srtTime(from), srtTime(to), l)
		}
	}
	return sb.String()
}

func srtTime(ms int64) string {
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}

// SafeName turns a title or file name into a short file-system-safe name.
func SafeName(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
		if b.Len() >= 80 {
			break
		}
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" {
		return "media"
	}
	return out
}

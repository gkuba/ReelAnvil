// Package pipeline holds the pure parts of the media pipeline: cutting audio
// into transcription segments, building and checking model prompts, choosing
// clips and writing captions. It does no I/O.
package pipeline

import (
	"encoding/json"
	"fmt"
	"slices"
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

// Lesson is something the user taught from a reviewed clip: Kind "good" is a
// clip they liked, "fix" a problem to avoid.
type Lesson struct{ Kind, Title, Note string }

// PromptLessons is how many of the newest lessons go into a prompt.
const PromptLessons = 12

// lessonsText writes the lessons as a prompt section, or "" when there are
// none. Notes are cut short so the section stays small.
func lessonsText(lessons []Lesson) string {
	var liked, avoid []string
	for _, l := range lessons[:min(len(lessons), PromptLessons)] {
		note := oneLine(l.Note, 200)
		switch l.Kind {
		case "good":
			line := oneLine(l.Title, 120)
			if note != "" {
				line += " — " + note
			}
			if line != "" {
				liked = append(liked, "- "+line)
			}
		case "fix":
			if note != "" {
				avoid = append(avoid, "- "+note)
			}
		}
	}
	var sb strings.Builder
	if len(liked) > 0 {
		sb.WriteString("\nClips the user liked:\n" + strings.Join(liked, "\n") + "\n")
	}
	if len(avoid) > 0 {
		sb.WriteString("\nProblems the user asked to avoid:\n" + strings.Join(avoid, "\n") + "\n")
	}
	return sb.String()
}

// HunterPrompt asks for clip proposals inside segs[from..to].
func HunterPrompt(b Brief, segs []Segment, from, to, want, minS, maxS int, lessons []Lesson) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, `You pick short vertical video clips from a transcript.
%s

Find up to %d moments that work on their own: a strong opening line (hook), a clear point, and an ending that pays it off. Each clip must be %d to %d seconds long. Clips must not overlap. End a clip only where the speakers have finished the whole answer or exchange, never while someone is still talking.
%s
%s
Transcript lines are numbered: [number] start-end text.
`, b.text(), want, minS, maxS, NotAClip, lessonsText(lessons))
	writeLines(&sb, segs, from, to)
	sb.WriteString(hunterReply)
	return sb.String()
}

const hunterReply = `
Reply with JSON only, in this form:
{"clips":[{"start":<first line number>,"end":<last line number>,"title":"<short title>","hook":"<the opening line>","payoff":"<how it ends>","why":"<why it works>","strength":<1-10>}]}`

func writeLines(sb *strings.Builder, segs []Segment, from, to int) {
	for i := from; i <= to; i++ {
		fmt.Fprintf(sb, "[%d] %s-%s %s\n", i, Clock(segs[i].StartMS), Clock(segs[i].EndMS), segs[i].Text)
	}
}

// FixPrompt asks for ONE clip inside segs[from..to] that fixes the problem in
// note. origStart..origEnd are the line numbers of the clip that was turned
// down. The reply has the hunter's form, so ParseHunt checks it.
func FixPrompt(b Brief, segs []Segment, from, to, origStart, origEnd int, note string, minS, maxS int, lessons []Lesson) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, `You fix a short vertical video clip that was turned down. The clips are cut from a transcript.
%s

The turned-down clip is lines %d-%d (%s-%s). The problem: %s

Pick ONE clip that fixes this problem. You may move its start and end, or pick a nearby moment that works better. The clip must be %d to %d seconds long and use only the numbered lines below. End a clip only where the speakers have finished the whole answer or exchange, never while someone is still talking.
%s
%s
Transcript lines are numbered: [number] start-end text.
`, b.text(), origStart, origEnd, Clock(segs[origStart].StartMS), Clock(segs[origEnd].EndMS), oneLine(note, 600), minS, maxS, NotAClip, lessonsText(lessons))
	writeLines(&sb, segs, from, to)
	sb.WriteString(hunterReply + "\nThe list must hold exactly one clip.")
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

// ReviewCriteria are the scores the reviewer gives, in the order they are
// shown and used to break ties.
var ReviewCriteria = []string{"hook", "complete", "standalone", "interest", "fit"}

// PartLabels are the labels the reviewer may give to lines of a clip.
var PartLabels = []string{"hook", "setup", "body", "payoff"}

// ReviewPrompt asks an independent reviewer to judge one candidate. It gets
// the numbered clip lines, the text around them and the brief, and nothing
// from the hunter's reasoning.
func ReviewPrompt(b Brief, segs []Segment, start, end int, lessons []Lesson) string {
	before := Text(segs, start-ReviewContext, start-1)
	after := Text(segs, end+1, end+ReviewContext)
	sp := Span{segs[start].StartMS, segs[end].EndMS}
	var lines strings.Builder
	for i := start; i <= end; i++ {
		fmt.Fprintf(&lines, "[%d] %s\n", i, segs[i].Text)
	}
	return fmt.Sprintf(`You review a short vertical video clip cut from a longer video, before a person sees it.
%s

Score the clip from 1 (very poor) to 10 (excellent) on each criterion:
- hook: the first seconds make a viewer keep watching.
- complete: it starts at the start of a thought and runs until the speaker has finished.
- standalone: it makes sense without the rest of the video.
- interest: it is interesting to most viewers, not only insiders.
- fit: it matches the campaign requirements (10 when there is no campaign brief).
Give each score a short reason. Label the parts of the clip by line number: hook, setup, body, payoff. Accept the clip only if every score is high.
%s
%s
Before the clip: %s
Clip (%d seconds), numbered lines:
%sAfter the clip: %s

Reply with JSON only: {"scores":{"hook":<1-10>,"complete":<1-10>,"standalone":<1-10>,"interest":<1-10>,"fit":<1-10>},"reasons":{"hook":"<short phrase>","complete":"<short phrase>","standalone":"<short phrase>","interest":"<short phrase>","fit":"<short phrase>"},"parts":[{"part":"hook","line":<line number>},{"part":"setup","line":<line number>},{"part":"body","line":<line number>},{"part":"payoff","line":<line number>}],"verdict":"accept" or "reject","reason":"<one sentence>"}`,
		b.text(), NotAClip, lessonsText(lessons), orNone(before), (sp.EndMS-sp.StartMS)/1000, lines.String(), orNone(after))
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(nothing)"
	}
	return s
}

// Part marks where a part of the clip begins, as a transcript line number.
type Part struct {
	Part string `json:"part"`
	Line int    `json:"line"`
}

// Review is a reviewer's judgement of one clip.
type Review struct {
	Verdict, Reason string
	Scores          map[string]int
	Reasons         map[string]string
	Parts           []Part
}

// ParseReview reads a reviewer reply about the clip made of lines start to
// end. All five scores must be whole numbers from 1 to 10; otherwise the
// caller should ask again. Invalid parts are dropped.
func ParseReview(raw string, start, end int) (Review, error) {
	var r struct {
		Verdict string            `json:"verdict"`
		Reason  string            `json:"reason"`
		Scores  map[string]any    `json:"scores"`
		Reasons map[string]string `json:"reasons"`
		Parts   []struct {
			Part string `json:"part"`
			Line any    `json:"line"`
		} `json:"parts"`
	}
	if err := json.Unmarshal([]byte(extractJSON(raw)), &r); err != nil {
		return Review{}, fmt.Errorf("the reply was not JSON")
	}
	v := strings.ToLower(strings.TrimSpace(r.Verdict))
	if v != "accept" && v != "reject" {
		return Review{}, fmt.Errorf("unknown verdict %q", r.Verdict)
	}
	out := Review{Verdict: v, Reason: oneLine(r.Reason, 500), Scores: map[string]int{}, Reasons: map[string]string{}}
	for _, k := range ReviewCriteria {
		raw, ok := r.Scores[k]
		if !ok {
			return Review{}, fmt.Errorf("the %s score is missing", k)
		}
		n, ok := toInt(raw)
		if !ok || n < 1 || n > 10 {
			return Review{}, fmt.Errorf("the %s score must be a whole number from 1 to 10", k)
		}
		out.Scores[k] = int(n)
		out.Reasons[k] = oneLine(r.Reasons[k], 120)
	}
	last := start - 1
	for _, p := range r.Parts {
		label := strings.ToLower(strings.TrimSpace(p.Part))
		n, ok := toInt(p.Line)
		if !ok || !slices.Contains(PartLabels, label) || int(n) < start || int(n) > end || int(n) <= last {
			continue
		}
		last = int(n)
		out.Parts = append(out.Parts, Part{label, last})
	}
	return out, nil
}

// Gate turns an accept into a reject when any score is below min. The reason
// then names the lowest criterion, which is the note an automatic retry uses.
func Gate(r Review, min int) Review {
	low := ""
	for _, k := range ReviewCriteria {
		if r.Scores[k] < min && (low == "" || r.Scores[k] < r.Scores[low]) {
			low = k
		}
	}
	if low == "" {
		return r
	}
	r.Verdict = "reject"
	r.Reason = fmt.Sprintf("%s %d/10", strings.ToUpper(low[:1])+low[1:], r.Scores[low])
	if r.Reasons[low] != "" {
		r.Reason += ": " + r.Reasons[low]
	}
	return r
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

// EndLimit returns the last line a clip starting at start may use: the
// longest it can run is the job's maximum or FinishCapS, whichever is more.
func EndLimit(segs []Segment, start, maxS int) int {
	limit := int64(max(maxS, FinishCapS)) * 1000
	last := start
	for last+1 < len(segs) && segs[last+1].EndMS-segs[start].StartMS <= limit {
		last++
	}
	return last
}

// EndPrompt asks where the answer in segs[start..] finishes. The clip was
// first cut at end; the speaker may stop earlier or go on.
func EndPrompt(b Brief, segs []Segment, start, end, last int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, `You check where a short vertical video clip should stop.
%s

The clip starts at line %d and was first cut at line %d. Find the line where the speaker finishes the whole answer, story or point that began at line %d. The clip must not stop while the point is still being made, and it must not run on into a new topic.
Transcript lines are numbered: [number] start-end text.
`, b.text(), start, end, start)
	writeLines(&sb, segs, start, last)
	sb.WriteString(`
Reply with JSON only: {"end":<line number where the point is finished>}`)
	return sb.String()
}

// ParseEnd reads the checked end line. It must lie in [start, last] and keep
// the clip at least minS seconds long; otherwise ok is false and the first
// cut stays.
func ParseEnd(raw string, segs []Segment, start, last, minS int) (end int, ok bool) {
	var reply struct {
		End any `json:"end"`
	}
	if err := json.Unmarshal([]byte(extractJSON(raw)), &reply); err != nil {
		return 0, false
	}
	e, good := toInt(reply.End)
	if !good || e < int64(start) || e > int64(last) {
		return 0, false
	}
	if (segs[e].EndMS-segs[start].StartMS)/1000 < int64(minS) {
		return 0, false
	}
	return int(e), true
}

// SettleEnd applies a checked end line, then moves it on to a sentence end.
func SettleEnd(segs []Segment, start, end, last, maxS int) int {
	return finishSentence(segs, start, end, last, maxS)
}

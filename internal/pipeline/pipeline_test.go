package pipeline

import (
	"fmt"
	"strings"
	"testing"
)

func TestCutUsesSilencesAndHardCuts(t *testing.T) {
	// Silences at 7.5-8.5 s and 15-16 s; nothing after 16 s.
	spans := Cut(40000, []Span{{7500, 8500}, {15000, 16000}})
	want := []Span{{0, 8000}, {8000, 15500}, {15500, 27500}, {27500, 39500}, {39500, 40000}}
	if len(spans) != len(want) {
		t.Fatalf("spans = %v", spans)
	}
	for i := range want {
		if spans[i] != want[i] {
			t.Fatalf("span %d = %v, want %v (all %v)", i, spans[i], want[i], spans)
		}
	}
	if got := Cut(5000, nil); len(got) != 1 || got[0] != (Span{0, 5000}) {
		t.Fatalf("short = %v", got)
	}
}

func segs(n int, each int64) []Segment {
	out := make([]Segment, n)
	for i := range out {
		out[i] = Segment{int64(i) * each, int64(i+1) * each, "word word word"}
	}
	return out
}

func TestParseHuntValidatesRangesLengthsAndOverlap(t *testing.T) {
	s := segs(20, 8000)
	raw := "Sure! " + `{"clips":[
		{"start":0,"end":2,"title":"A","strength":8},
		{"start":2,"end":4,"title":"overlap"},
		{"start":5,"end":5,"title":"short, extended to fit"},
		{"start":18,"end":25,"title":"outside"},
		{"start":10,"end":19,"title":"","strength":"x"},
		{"start":19,"end":19,"title":"short, no room left"}]}`
	picks, problems := ParseHunt(raw, s, 0, 19, 15, 60)
	if len(picks) != 3 || picks[0].Title != "A" || picks[1].Start != 5 || picks[1].End != 6 ||
		picks[2].Start != 10 || picks[2].End != 16 || picks[2].Strength != 5 || !strings.HasPrefix(picks[2].Title, "Clip at") {
		t.Fatalf("picks = %+v", picks)
	}
	if len(problems) != 3 {
		t.Fatalf("problems = %v", problems)
	}
	if _, p := ParseHunt("no json", s, 0, 19, 15, 60); len(p) != 1 {
		t.Fatalf("bad json problems = %v", p)
	}
}

// Regression: a clip ended on a line cut at a pause mid-sentence
// ("...humble and awestruck at what..."). Clips grow to finish the sentence,
// past the job's maximum up to FinishCapS, and are never cut shorter.
func TestFinishSentenceOnlyExtends(t *testing.T) {
	s := segs(20, 10000)
	s[3].Text, s[4].Text = "a full sentence.", "humble and awestruck at what..."
	s[7].Text = "we built up there."
	// 50 s clip, max 55: grows to line 7 (80 s), not back to line 3.
	if got := finishSentence(s, 0, 4, 19, 55); got != 7 {
		t.Fatalf("got %d, want 7", got)
	}
	// Already ends a sentence: unchanged.
	if got := finishSentence(s, 0, 3, 19, 55); got != 3 {
		t.Fatalf("got %d, want 3", got)
	}
	// The sentence ends at 130 s, past the 120 s cap: keep the end as chosen.
	s[7].Text, s[12].Text = "and then", "finally."
	if got := finishSentence(s, 0, 4, 19, 55); got != 4 {
		t.Fatalf("past cap = %d, want 4", got)
	}
	// A job maximum above the cap is the limit instead.
	if got := finishSentence(s, 0, 4, 19, 180); got != 12 {
		t.Fatalf("job max = %d, want 12", got)
	}
}

func TestBestKeepsStrongestWithoutOverlap(t *testing.T) {
	got := Best([]Pick{{Start: 0, End: 2, Strength: 3}, {Start: 1, End: 3, Strength: 9}, {Start: 5, End: 6, Strength: 4}, {Start: 8, End: 9, Strength: 1}}, 2)
	if len(got) != 2 || got[0].Start != 1 || got[1].Start != 5 {
		t.Fatalf("best = %+v", got)
	}
}

func TestReviewPromptHidesHunterReasoning(t *testing.T) {
	s := segs(6, 8000)
	s[2].Text = "the clip text"
	p := ReviewPrompt(Brief{Title: "Camp"}, s, 2, 3, nil)
	if !strings.Contains(p, "the clip text") || !strings.Contains(p, "Camp") {
		t.Fatal(p)
	}
	if !strings.Contains(p, NotAClip) {
		t.Fatal("reviewer prompt lacks the not-a-clip rule")
	}
	if h := HunterPrompt(Brief{}, s, 0, 5, 3, 20, 60, nil); !strings.Contains(h, NotAClip) || !strings.Contains(h, "[5] ") {
		t.Fatal("hunter prompt lacks the not-a-clip rule or lines")
	}
	if !strings.Contains(p, "[2] the clip text") || !strings.Contains(p, "[3] ") || !strings.Contains(p, "fit: ") {
		t.Fatalf("review prompt lacks numbered lines or criteria:\n%s", p)
	}
}

const goodReview = `{"scores":{"hook":8,"complete":9,"standalone":7,"interest":6,"fit":10},"reasons":{"hook":"strong question","complete":"ends cleanly","standalone":"ok","interest":"niche","fit":"no brief"},"parts":[{"part":"hook","line":2},{"part":"payoff","line":3}],"verdict":"Accept","reason":"good"}`

func TestParseReview(t *testing.T) {
	r, err := ParseReview(goodReview, 2, 3)
	if err != nil || r.Verdict != "accept" || r.Reason != "good" || r.Scores["interest"] != 6 || r.Reasons["hook"] != "strong question" {
		t.Fatalf("%+v %v", r, err)
	}
	if len(r.Parts) != 2 || r.Parts[1] != (Part{"payoff", 3}) {
		t.Fatalf("parts = %+v", r.Parts)
	}
	for name, raw := range map[string]string{
		"unknown verdict": `{"verdict":"maybe"}`,
		"missing score":   `{"scores":{"hook":8,"complete":9,"standalone":7,"interest":6},"verdict":"accept"}`,
		"score too high":  `{"scores":{"hook":11,"complete":9,"standalone":7,"interest":6,"fit":5},"verdict":"accept"}`,
		"score zero":      `{"scores":{"hook":0,"complete":9,"standalone":7,"interest":6,"fit":5},"verdict":"accept"}`,
		"score fraction":  `{"scores":{"hook":7.5,"complete":9,"standalone":7,"interest":6,"fit":5},"verdict":"accept"}`,
		"old reply form":  `{"verdict":"accept","reason":"good"}`,
		"not json":        `nope`,
	} {
		if _, err = ParseReview(raw, 2, 3); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestParseReviewDropsInvalidParts(t *testing.T) {
	raw := `{"scores":{"hook":8,"complete":9,"standalone":7,"interest":6,"fit":5},"verdict":"reject","reason":"r","reasons":{"hook":"` + strings.Repeat("x", 300) + `"},
		"parts":[{"part":"intro","line":4},{"part":"setup","line":4},{"part":"body","line":9},{"part":"hook","line":3},{"part":"payoff","line":"6"},{"part":"payoff","line":5},{"part":"body","line":5}]}`
	r, err := ParseReview(raw, 4, 6)
	if err != nil {
		t.Fatal(err)
	}
	// Unknown label, line outside the clip and out-of-order lines are dropped;
	// the first kept part need not be the hook.
	want := []Part{{"setup", 4}, {"payoff", 6}}
	if len(r.Parts) != len(want) || r.Parts[0] != want[0] || r.Parts[1] != want[1] {
		t.Fatalf("parts = %+v", r.Parts)
	}
	if n := len([]rune(r.Reasons["hook"])); n > 121 {
		t.Errorf("reason has %d runes", n)
	}
}

func TestGate(t *testing.T) {
	r, _ := ParseReview(goodReview, 2, 3)
	if g := Gate(r, 6); g.Verdict != "accept" || g.Reason != "good" {
		t.Fatalf("all clear changed: %+v", g)
	}
	g := Gate(r, 7)
	if g.Verdict != "reject" || g.Reason != "Interest 6/10: niche" {
		t.Fatalf("gated: %+v", g)
	}
	// Ties go in criteria order: hook before complete.
	r.Scores["hook"], r.Scores["complete"], r.Scores["interest"] = 3, 3, 9
	if g = Gate(r, 6); g.Reason != "Hook 3/10: strong question" {
		t.Fatalf("tie: %q", g.Reason)
	}
	// A model reject stays a reject with its own reason when scores are fine.
	r.Scores["hook"], r.Scores["complete"], r.Verdict, r.Reason = 9, 9, "reject", "dull"
	if g = Gate(r, 6); g.Verdict != "reject" || g.Reason != "dull" {
		t.Fatalf("model reject changed: %+v", g)
	}
}

func TestSRTIsClipRelative(t *testing.T) {
	s := []Segment{{0, 4000, "before"}, {10000, 16000, "one two three four five six seven eight nine ten eleven twelve"}, {20000, 22000, "after"}}
	out := SRT(s, Span{10000, 16000})
	if !strings.HasPrefix(out, "1\n00:00:00,000 --> 00:00:0") || strings.Contains(out, "before") || strings.Contains(out, "after") {
		t.Fatal(out)
	}
	if !strings.Contains(out, "2\n00:00:02,655") || !strings.Contains(out, "--> 00:00:06,000\nseven") {
		t.Fatal(out)
	}
}

func TestWindowsAndSafeName(t *testing.T) {
	w := Windows(segs(10, 1000), 3*(14+24))
	if len(w) != 4 || w[0] != [2]int{0, 2} || w[3] != [2]int{9, 9} {
		t.Fatalf("windows = %v", w)
	}
	if got := SafeName("../My Video (1).MP4"); got != "my-video-1-.mp4" && got != "my-video-1.mp4" {
		t.Fatalf("safe = %q", got)
	}
	if SafeName("///") != "media" {
		t.Fatal("empty name")
	}
}

var testLessons = []Lesson{
	{Kind: "good", Title: "The origin story", Note: "short and funny"},
	{Kind: "fix", Title: "Sound check", Note: "started mid-sentence"},
}

func TestPromptsIncludeLessonsOnlyWhenThereAreSome(t *testing.T) {
	s := segs(6, 8000)
	for name, p := range map[string]string{
		"hunter":   HunterPrompt(Brief{}, s, 0, 5, 3, 20, 60, testLessons),
		"reviewer": ReviewPrompt(Brief{}, s, 2, 3, testLessons),
		"fix":      FixPrompt(Brief{}, s, 0, 5, 2, 3, "too long", 20, 60, testLessons),
	} {
		for _, want := range []string{"Clips the user liked:", "- The origin story — short and funny", "Problems the user asked to avoid:", "- started mid-sentence"} {
			if !strings.Contains(p, want) {
				t.Errorf("%s prompt lacks %q", name, want)
			}
		}
		if strings.Contains(p, "Sound check") {
			t.Errorf("%s prompt shows a fix lesson's title", name)
		}
	}
	for name, p := range map[string]string{
		"hunter":   HunterPrompt(Brief{}, s, 0, 5, 3, 20, 60, nil),
		"reviewer": ReviewPrompt(Brief{}, s, 2, 3, nil),
		"fix":      FixPrompt(Brief{}, s, 0, 5, 2, 3, "too long", 20, 60, nil),
	} {
		if strings.Contains(p, "the user liked") || strings.Contains(p, "asked to avoid") {
			t.Errorf("%s prompt has a lessons section without lessons", name)
		}
	}
}

func TestLessonsUseTheNewestTwelveWithShortNotes(t *testing.T) {
	var ls []Lesson
	for i := 0; i < 15; i++ {
		ls = append(ls, Lesson{Kind: "fix", Note: fmt.Sprintf("note-%02d ", i) + strings.Repeat("x", 300)})
	}
	text := lessonsText(ls)
	if !strings.Contains(text, "note-11") || strings.Contains(text, "note-12") {
		t.Errorf("lessons not cut to 12: %s", text)
	}
	if strings.Contains(text, strings.Repeat("x", 250)) {
		t.Error("note not cut to 200 characters")
	}
}

func TestFixPromptNamesTheNoteAndTheOriginalRange(t *testing.T) {
	s := segs(30, 8000)
	p := FixPrompt(Brief{Title: "Camp"}, s, 5, 25, 10, 14, "it stops mid-answer", 20, 60, nil)
	for _, want := range []string{"it stops mid-answer", "lines 10-14", Clock(s[10].StartMS) + "-" + Clock(s[14].EndMS), "[5] ", "[25] ", "20 to 60 seconds", "exactly one clip", "Camp"} {
		if !strings.Contains(p, want) {
			t.Errorf("fix prompt lacks %q", want)
		}
	}
	if strings.Contains(p, "[4] ") || strings.Contains(p, "[26] ") {
		t.Error("fix prompt shows lines outside the window")
	}
	// The reply is checked by ParseHunt inside the window.
	picks, problems := ParseHunt(`{"clips":[{"start":6,"end":9,"title":"Fixed","strength":8}]}`, s, 5, 25, 20, 60)
	if len(picks) != 1 || len(problems) != 0 || picks[0].Title != "Fixed" {
		t.Errorf("picks %+v problems %v", picks, problems)
	}
}

func TestParseEndChecksRangeAndLength(t *testing.T) {
	s := segs(30, 5000) // 5 s lines
	if e, ok := ParseEnd(`{"end": 9}`, s, 2, 20, 20); !ok || e != 9 {
		t.Fatalf("end = %d %v", e, ok)
	}
	for _, raw := range []string{`{"end": 21}`, `{"end": 1}`, `{"end": 3}`, `nope`, `{"end":"x"}`} {
		if _, ok := ParseEnd(raw, s, 2, 20, 20); ok {
			t.Fatalf("%s accepted", raw)
		}
	}
	if got := EndLimit(s, 0, 60); got != 23 { // 120 s cap = 24 lines (0..23)
		t.Fatalf("limit = %d", got)
	}
	if !strings.Contains(EndPrompt(Brief{}, s, 2, 6, 10), "[10]") {
		t.Fatal("prompt misses the last line")
	}
}

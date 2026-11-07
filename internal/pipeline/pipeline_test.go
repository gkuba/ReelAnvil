package pipeline

import (
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
	p := ReviewPrompt(Brief{Title: "Camp"}, s, 2, 3)
	if !strings.Contains(p, "the clip text") || !strings.Contains(p, "Camp") {
		t.Fatal(p)
	}
	if !strings.Contains(p, NotAClip) {
		t.Fatal("reviewer prompt lacks the not-a-clip rule")
	}
	if h := HunterPrompt(Brief{}, s, 0, 5, 3, 20, 60); !strings.Contains(h, NotAClip) || !strings.Contains(h, "[5] ") {
		t.Fatal("hunter prompt lacks the not-a-clip rule or lines")
	}
	v, r, err := ParseReview(`{"verdict":"Accept","reason":"good"}`)
	if err != nil || v != "accept" || r != "good" {
		t.Fatalf("%q %q %v", v, r, err)
	}
	if _, _, err = ParseReview(`{"verdict":"maybe"}`); err == nil {
		t.Fatal("accepted unknown verdict")
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

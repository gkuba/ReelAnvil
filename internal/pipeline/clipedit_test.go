package pipeline

import (
	"strings"
	"testing"
)

func TestWordCuesSplitAndTimeInsideLine(t *testing.T) {
	segs := []Segment{{0, 6000, "one two three four five six"}, {6000, 8000, "seven eight"}}
	cues := WordCues(segs, Span{0, 8000}, 3)
	if len(cues) != 3 || cues[0].Text != "one two three" || cues[1].Text != "four five six" || cues[2].Text != "seven eight" {
		t.Fatalf("cues = %+v", cues)
	}
	if cues[1].To != 6000 || cues[0].To <= 0 || cues[0].To >= 6000 || cues[2].From != 6000 || cues[2].To != 8000 {
		t.Fatalf("times = %+v", cues)
	}
	if got := WordCues(segs, Span{6000, 8000}, 3); len(got) != 1 {
		t.Fatalf("clip span not honoured: %+v", got)
	}
}

func TestFillerSpansOnlyFillerLines(t *testing.T) {
	segs := []Segment{{0, 500, "Um."}, {500, 4000, "Um, I think so."}, {4000, 4400, "Uh, uh."}, {4400, 5000, "Hmm"}, {5000, 6000, "Humble pie."}}
	got := FillerSpans(segs)
	if len(got) != 3 || got[0] != (Span{0, 500}) || got[1] != (Span{4000, 4400}) || got[2] != (Span{4400, 5000}) {
		t.Fatalf("fillers = %v", got)
	}
}

func TestKeepSpansCutsLongSilencesAndFillers(t *testing.T) {
	clip := Span{10000, 40000}
	keep := KeepSpans(clip, []Span{{15000, 16500}, {20000, 20400}, {38000, 40500}}, []Span{{25000, 25600}})
	want := []Span{{10000, 15200}, {16300, 25000}, {25600, 38200}}
	if len(keep) != len(want) {
		t.Fatalf("keep = %v", keep)
	}
	for i := range want {
		if keep[i] != want[i] {
			t.Fatalf("keep = %v, want %v", keep, want)
		}
	}
	if got := KeepSpans(clip, []Span{{20000, 20400}}, nil); len(got) != 1 || got[0] != clip {
		t.Fatalf("nothing to cut: %v", got)
	}
	// A silence at the clip's start is trimmed to the pad.
	if got := KeepSpans(clip, []Span{{9000, 12000}}, nil); got[0].StartMS != 11800 {
		t.Fatalf("leading silence: %v", got)
	}
}

func TestRemapAndCaptionsFollowCuts(t *testing.T) {
	keep := []Span{{0, 5000}, {8000, 12000}}
	for in, want := range map[int64]int64{0: 0, 4000: 4000, 6000: 5000, 8000: 5000, 9000: 6000, 12000: 9000, 99999: 9000} {
		if got := Remap(keep, in); got != want {
			t.Errorf("Remap(%d) = %d, want %d", in, got, want)
		}
	}
	cues := []Cue{{1000, 3000, "kept"}, {5500, 7500, "inside the cut"}, {9000, 11000, "later"}}
	srt := CuesSRT(cues, keep)
	if strings.Contains(srt, "inside the cut") || !strings.Contains(srt, "00:00:06,000 --> 00:00:08,000\nlater") {
		t.Fatalf("srt:\n%s", srt)
	}
	ass := ASS(cues, "My {title}", keep)
	for _, want := range []string{"PlayResX: 1080", "Dialogue: 1,0:00:00.00,0:00:02.50,Title,,0,0,0,,My (title)", "Dialogue: 0,0:00:06.00,0:00:08.00,Default,,0,0,0,,later"} {
		if !strings.Contains(ass, want) {
			t.Errorf("ass missing %q:\n%s", want, ass)
		}
	}
	if strings.Contains(ASS(cues, "", keep), "Title,,") {
		t.Error("title line without a title")
	}
}

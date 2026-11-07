package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// buildV4Database creates a schema-v4 database the way the previous build did,
// with one job, candidate and clip.
func buildV4Database(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "v4.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range migrations[:4] {
		if err = m(ctx, tx); err != nil {
			t.Fatal(err)
		}
	}
	for _, stmt := range []string{
		`INSERT INTO jobs(id,title,origin,origin_ref,state,clip_count,min_s,max_s,rights_confirmed_at,created_at,updated_at)
			VALUES(7,'Old job','file','a.mp4','completed',2,20,60,'2026-10-01T10:00:00Z','2026-10-01T10:00:00Z','2026-10-01T10:00:00Z')`,
		`INSERT INTO candidates(id,job_id,ord,start_ms,end_ms,title,hook,payoff,why,strength,text,model,created_at)
			VALUES(3,7,1,0,30000,'Kept','h','p','w',8,'text','m','2026-10-01T10:00:00Z')`,
		`INSERT INTO clips(id,job_id,candidate_id,path,srt_path,captions,qa_pass,decision,note,created_at)
			VALUES(5,7,3,'completed/7/a.mp4','completed/7/a.srt','burned',1,'approved','my note','2026-10-01T10:00:00Z')`,
		`PRAGMA user_version=4`,
	} {
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMigrateV4ToV5KeepsDataAndAddsLessonFields(t *testing.T) {
	s, err := Open(buildV4Database(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	var version int
	if err = s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != SchemaVersion || SchemaVersion < 5 {
		t.Fatalf("user_version = %d (%v), want %d", version, err, SchemaVersion)
	}
	j, err := s.JobByID(ctx, 7)
	if err != nil || j.Title != "Old job" || j.ClipCount != 2 || j.Attempt != 1 || j.RedoClipID != 0 || j.RedoNote != "" {
		t.Fatalf("job after migration = %+v %v", j, err)
	}
	cands, err := s.ListCandidates(ctx, 7)
	if err != nil || len(cands) != 1 || cands[0].Title != "Kept" || cands[0].Attempt != 1 {
		t.Fatalf("candidates after migration = %+v %v", cands, err)
	}
	c, err := s.ClipByID(ctx, 5)
	if err != nil || c.Decision != DecisionApproved || c.Note != "my note" || c.Attempt != 1 {
		t.Fatalf("clip after migration = %+v %v", c, err)
	}
	if err = s.DecideClip(ctx, 5, DecisionApproved, "good one", true); err != nil {
		t.Fatal(err)
	}
	if ls, err := s.ListLessons(ctx, 10); err != nil || len(ls) != 1 || ls[0].Kind != LessonGood || ls[0].Title != "Kept" {
		t.Fatalf("lessons = %+v %v", ls, err)
	}
}

// reviewedClip stores a job with one passing clip and returns the clip's ID.
func reviewedClip(t *testing.T, s *Store, title string) (jobID, clipID int64) {
	t.Helper()
	ctx := context.Background()
	jobID, err := s.CreateJob(ctx, JobInput{Title: "job", Origin: "file", Ref: "a.mp4", ClipCount: 2, MinS: 20, MaxS: 60})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ReplaceCandidates(ctx, jobID, []Candidate{{StartMS: 8000, EndMS: 40000, Title: title, Strength: 7, Text: "x"}}); err != nil {
		t.Fatal(err)
	}
	cands, _ := s.ListCandidates(ctx, jobID)
	if clipID, err = s.AddClip(ctx, Clip{JobID: jobID, CandidateID: cands[0].ID, Path: "p", SRTPath: "q", Captions: "burned"}); err != nil {
		t.Fatal(err)
	}
	if err = s.SetClipQA(ctx, Clip{ID: clipID, Path: "p", SRTPath: "q", QAPass: true}); err != nil {
		t.Fatal(err)
	}
	return jobID, clipID
}

func TestDecideClipFeedbackUpsertsAndDeletesLessons(t *testing.T) {
	s := openCampaignStore(t)
	ctx := context.Background()
	_, clip := reviewedClip(t, s, "Origin story")
	lessons := func() []Lesson {
		t.Helper()
		ls, err := s.ListLessons(ctx, 10)
		if err != nil {
			t.Fatal(err)
		}
		return ls
	}

	// Feedback off: the note is kept for the user only.
	if err := s.DecideClip(ctx, clip, DecisionApproved, "nice", false); err != nil || len(lessons()) != 0 {
		t.Fatalf("lessons without feedback: %v %v", lessons(), err)
	}
	// Approved with feedback: a good lesson, note optional.
	if err := s.DecideClip(ctx, clip, DecisionApproved, "", true); err != nil {
		t.Fatal(err)
	}
	if ls := lessons(); len(ls) != 1 || ls[0].Kind != LessonGood || ls[0].Title != "Origin story" || ls[0].Note != "" || ls[0].ClipID != clip {
		t.Fatalf("good lesson = %+v", ls)
	}
	// Rejecting without a note cannot teach anything, and changes nothing.
	err := s.DecideClip(ctx, clip, DecisionRejected, "  ", true)
	if _, ok := err.(InputError); !ok || !strings.Contains(err.Error(), "Add a note saying what to fix.") {
		t.Fatalf("reject without note = %v", err)
	}
	if c, _ := s.ClipByID(ctx, clip); c.Decision != DecisionApproved {
		t.Fatalf("decision changed to %s by a refused request", c.Decision)
	}
	// Rejected with feedback replaces the lesson: still one per clip, notes cut to 300.
	if err = s.DecideClip(ctx, clip, DecisionRejected, strings.Repeat("é", 400), true); err != nil {
		t.Fatal(err)
	}
	ls := lessons()
	if len(ls) != 1 || ls[0].Kind != LessonFix || len([]rune(ls[0].Note)) != LessonNoteMax {
		t.Fatalf("fix lesson = %+v", ls)
	}
	if c, _ := s.ClipByID(ctx, clip); len([]rune(c.Note)) != 400 {
		t.Fatal("the clip's own note was cut")
	}
	// Back to pending, or feedback off, removes it.
	if err = s.DecideClip(ctx, clip, DecisionRejected, "again", true); err != nil {
		t.Fatal(err)
	}
	if err = s.DecideClip(ctx, clip, DecisionPending, "again", true); err != nil || len(lessons()) != 0 {
		t.Fatalf("pending kept a lesson: %v %v", lessons(), err)
	}
	if err = s.DecideClip(ctx, clip, DecisionApproved, "", true); err != nil {
		t.Fatal(err)
	}
	if err = s.DecideClip(ctx, clip, DecisionApproved, "", false); err != nil || len(lessons()) != 0 {
		t.Fatalf("feedback off kept a lesson: %v %v", lessons(), err)
	}
	// DeleteLesson.
	if err = s.DecideClip(ctx, clip, DecisionApproved, "", true); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteLesson(ctx, lessons()[0].ID); err != nil || len(lessons()) != 0 {
		t.Fatalf("delete: %v %v", lessons(), err)
	}
	if err = s.DeleteLesson(ctx, 999); err != ErrNotFound {
		t.Fatalf("delete of a missing lesson = %v", err)
	}
}

func TestCreateRedoJobCopiesTranscriptAndMarksStepsDone(t *testing.T) {
	s := openCampaignStore(t)
	ctx := context.Background()
	camp, _, err := s.AddManualCampaign(ctx, CampaignInput{Platform: "vyro", URL: "https://example.com/c", Title: "Brief"})
	if err != nil {
		t.Fatal(err)
	}
	jobID, err := s.CreateJob(ctx, JobInput{Title: "talk", Origin: "file", Ref: "a.mp4", CampaignID: camp.ID, ClipCount: 3, MinS: 25, MaxS: 70})
	if err != nil {
		t.Fatal(err)
	}
	src, err := s.AddSource(ctx, Source{Title: "talk", Origin: "file", Ref: "a.mp4", Path: "archive/a.mp4", RightsAt: "2026-10-01T10:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetJobSource(ctx, jobID, src); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveTranscript(ctx, jobID, "whisper", "en", []Segment{{0, 8000, "one"}, {8000, 16000, "two"}}); err != nil {
		t.Fatal(err)
	}
	if err = s.ReplaceCandidates(ctx, jobID, []Candidate{{StartMS: 0, EndMS: 16000, Title: "First try", Strength: 6, Text: "x"}}); err != nil {
		t.Fatal(err)
	}
	cands, _ := s.ListCandidates(ctx, jobID)
	clip, err := s.AddClip(ctx, Clip{JobID: jobID, CandidateID: cands[0].ID, Path: "p", SRTPath: "q", Captions: "burned"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateRedoJob(ctx, clip, "  "); err == nil {
		t.Fatal("redo job without a note")
	}

	redo, err := s.CreateRedoJob(ctx, clip, " starts too late ")
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.JobByID(ctx, redo)
	if err != nil {
		t.Fatal(err)
	}
	if j.Title != "First try — attempt 2" || j.Attempt != 2 || j.ClipCount != 1 || j.MinS != 25 || j.MaxS != 70 || j.SourceID != src ||
		j.CampaignID != camp.ID || j.RedoClipID != clip || j.RedoNote != "starts too late" || j.State != JobQueued || j.RightsAt == "" {
		t.Fatalf("redo job = %+v", j)
	}
	for _, name := range []string{"import", "transcribe"} {
		if st := j.Step(name); st.State != StepDone || st.Detail != "Reused from job 1" {
			t.Errorf("%s step = %+v", name, st)
		}
	}
	for _, name := range []string{"hunt", "review", "render", "qa"} {
		if st := j.Step(name); st.State != StepPending {
			t.Errorf("%s step = %+v", name, st)
		}
	}
	segs, err := s.Transcript(ctx, redo)
	if err != nil || len(segs) != 2 || segs[1].Text != "two" {
		t.Fatalf("copied transcript = %v %v", segs, err)
	}
	// A retry of the retry counts on, and the original clip cannot be deleted
	// from under the redo job.
	if err = s.DeletePendingClips(ctx, jobID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClipByID(ctx, clip); err != nil {
		t.Fatalf("a clip with a redo job was deleted: %v", err)
	}
	if _, err = s.CreateRedoJob(ctx, 999, "x"); err != ErrNotFound {
		t.Fatalf("redo of a missing clip = %v", err)
	}
}

func TestAddCandidateAppendsWithNextOrderAndAttempt(t *testing.T) {
	s := openCampaignStore(t)
	ctx := context.Background()
	jobID, _ := reviewedClip(t, s, "one")
	c, err := s.AddCandidate(ctx, jobID, Candidate{StartMS: 0, EndMS: 9000, Title: "two", Strength: 5, Text: "y", Attempt: 2})
	if err != nil || c.Ord != 2 || c.Attempt != 2 || c.ID == 0 {
		t.Fatalf("added = %+v %v", c, err)
	}
	cands, _ := s.ListCandidates(ctx, jobID)
	if len(cands) != 2 || cands[1].Title != "two" || cands[1].Attempt != 2 || cands[0].Attempt != 1 {
		t.Fatalf("candidates = %+v", cands)
	}
}

func TestMigrateV5ToV6KeepsDataAndStoresScores(t *testing.T) {
	path := buildV4Database(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = migrations[4](ctx, tx); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(ctx, `PRAGMA user_version=5`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cands, err := s.ListCandidates(ctx, 7)
	if err != nil || len(cands) != 1 || cands[0].Title != "Kept" || cands[0].Scores != "" || cands[0].Parts != "" {
		t.Fatalf("candidates after migration = %+v %v", cands, err)
	}
	if err = s.SaveReview(ctx, 3, "reject", "Hook 3/10: slow", "m", `{"scores":{"hook":3}}`, `[{"part":"hook","ms":0}]`); err != nil {
		t.Fatal(err)
	}
	cands, _ = s.ListCandidates(ctx, 7)
	if cands[0].Verdict != "reject" || cands[0].Scores != `{"scores":{"hook":3}}` || !strings.Contains(cands[0].Parts, "hook") {
		t.Fatalf("saved review = %+v", cands[0])
	}
	c, err := s.ClipByID(ctx, 5)
	if err != nil || c.Scores != cands[0].Scores || c.Parts != cands[0].Parts || c.Reason != "Hook 3/10: slow" {
		t.Fatalf("clip = %+v %v", c, err)
	}
}

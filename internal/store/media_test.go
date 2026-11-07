package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestJobLifecycleOpsClipsAndSubmissions(t *testing.T) {
	s := openCampaignStore(t)
	ctx := context.Background()
	camp, _, err := s.AddManualCampaign(ctx, CampaignInput{Platform: "vyro", URL: "https://example.com/c", Title: "Brief"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateJob(ctx, JobInput{Origin: "file", Ref: "a.mp4", ClipCount: 9, MinS: 15, MaxS: 60}); err == nil {
		t.Fatal("clip count 9 accepted")
	}
	id, err := s.CreateJob(ctx, JobInput{Title: "a", Origin: "file", Ref: "a.mp4", CampaignID: camp.ID, ClipCount: 2, MinS: 15, MaxS: 60})
	if err != nil {
		t.Fatal(err)
	}
	j, ok, err := s.NextQueuedJob(ctx)
	if err != nil || !ok || j.ID != id || len(j.Steps) != len(Steps) || j.CampaignTitle != "Brief" {
		t.Fatalf("next job = %+v %v %v", j, ok, err)
	}
	if err = s.SetJobState(ctx, id, JobRunning, "", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.SetStep(ctx, id, "import", StepRunning, ""); err != nil {
		t.Fatal(err)
	}

	opID, err := s.CreateOp(ctx, id, "import", "probe", map[string]string{"path": "/m/a.mp4"})
	if err != nil {
		t.Fatal(err)
	}
	op, ok, err := s.ClaimOp(ctx)
	if err != nil || !ok || op.ID != opID || op.Kind != "probe" {
		t.Fatalf("claim = %+v %v %v", op, ok, err)
	}
	if _, ok, _ = s.ClaimOp(ctx); ok {
		t.Fatal("claimed the same op twice")
	}
	if err = s.OpProgress(ctx, opID, 5, 10, "half"); err != nil {
		t.Fatal(err)
	}
	j, _ = s.JobByID(ctx, id)
	if st := j.Step("import"); st.Done != 5 || st.Total != 10 {
		t.Fatalf("step progress = %+v", st)
	}
	if err = s.FinishOp(ctx, opID, true, json.RawMessage(`{"ok":1}`), ""); err != nil {
		t.Fatal(err)
	}
	if err = s.OpProgress(ctx, opID, 6, 10, ""); !errors.Is(err, ErrOpCancelled) {
		t.Fatalf("progress after finish = %v", err)
	}

	// A stale claimed op fails; a restart fails the running job and keeps
	// finished steps for Retry.
	stale, _ := s.CreateOp(ctx, id, "import", "render", nil)
	if _, _, err = s.ClaimOp(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.FailStaleOps(ctx, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if o, _ := s.OpByID(ctx, stale); o.State != OpFailed {
		t.Fatalf("stale op state = %s", o.State)
	}
	if err = s.SetStep(ctx, id, "import", StepDone, ""); err != nil {
		t.Fatal(err)
	}
	if err = s.SetStep(ctx, id, "transcribe", StepRunning, ""); err != nil {
		t.Fatal(err)
	}
	if n, err := s.RecoverInterrupted(ctx); err != nil || n != 1 {
		t.Fatalf("recover = %d %v", n, err)
	}
	j, _ = s.JobByID(ctx, id)
	if j.State != JobFailed || j.Step("transcribe").State != StepFailed {
		t.Fatalf("after restart: %s %+v", j.State, j.Step("transcribe"))
	}
	if err = s.RetryJob(ctx, id); err != nil {
		t.Fatal(err)
	}
	j, _ = s.JobByID(ctx, id)
	if j.State != JobQueued || j.Step("import").State != StepDone || j.Step("transcribe").State != StepPending {
		t.Fatalf("after retry: %s %+v", j.State, j.Steps)
	}

	if err = s.SaveTranscript(ctx, id, "m", "en", []Segment{{0, 3000, "hello"}}); err != nil {
		t.Fatal(err)
	}
	if segs, err := s.Transcript(ctx, id); err != nil || len(segs) != 1 || segs[0].Text != "hello" {
		t.Fatalf("transcript = %v %v", segs, err)
	}
	if err = s.ReplaceCandidates(ctx, id, []Candidate{{StartMS: 0, EndMS: 20000, Title: "t", Strength: 7, Text: "x"}}); err != nil {
		t.Fatal(err)
	}
	cands, _ := s.ListCandidates(ctx, id)
	if len(cands) != 1 {
		t.Fatalf("candidates = %v", cands)
	}
	if err = s.SaveReview(ctx, cands[0].ID, "accept", "clear", "r"); err != nil {
		t.Fatal(err)
	}
	clipID, err := s.AddClip(ctx, Clip{JobID: id, CandidateID: cands[0].ID, Path: "temp/x.mp4", SRTPath: "temp/x.srt", Captions: "burned"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DecideClip(ctx, clipID, DecisionApproved, ""); err == nil {
		t.Fatal("approved a clip that has not passed QA")
	}
	if err = s.SetClipQA(ctx, Clip{ID: clipID, Path: "completed/1/x.mp4", SRTPath: "completed/1/x.srt", QAPass: true, Width: 1080, Height: 1920}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddSubmission(ctx, clipID, "https://example.com/p", "Submitted", ""); err == nil {
		t.Fatal("recorded a post for a clip that is not approved")
	}
	if err = s.DecideClip(ctx, clipID, DecisionApproved, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddSubmission(ctx, clipID, "http://example.com/p", "Submitted", ""); err == nil {
		t.Fatal("accepted a plain http post URL")
	}
	subID, err := s.AddSubmission(ctx, clipID, "https://example.com/p", "Submitted", "")
	if err != nil {
		t.Fatal(err)
	}
	views, earn := int64(1200), int64(350)
	if err = s.UpdateSubmission(ctx, subID, "Accepted", &views, &earn, "paid"); err != nil {
		t.Fatal(err)
	}
	clips, err := s.ListClips(ctx, DecisionApproved, 0)
	if err != nil || len(clips) != 1 || len(clips[0].Submissions) != 1 || clips[0].Verdict != "accept" || clips[0].CampaignTitle != "Brief" {
		t.Fatalf("clips = %+v %v", clips, err)
	}
	work, err := s.CampaignWork(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if w := work[camp.ID]; w.Jobs != 1 || w.Clips != 1 || w.Approved != 1 || w.Posts != 1 || w.Views != 1200 || w.EarningsCents != 350 {
		t.Fatalf("campaign work = %+v", w)
	}
	// Candidates cannot be replaced once clips exist.
	if err = s.ReplaceCandidates(ctx, id, nil); err == nil {
		t.Fatal("replaced candidates of a job with clips")
	}
}

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Job states.
const (
	JobQueued    = "queued"
	JobRunning   = "running"
	JobBlocked   = "blocked"
	JobFailed    = "failed"
	JobCancelled = "cancelled"
	JobCompleted = "completed"
)

// Step states.
const (
	StepPending = "pending"
	StepRunning = "running"
	StepDone    = "done"
	StepFailed  = "failed"
)

// Steps are the pipeline stages in run order.
var Steps = []string{"import", "transcribe", "hunt", "review", "render", "qa"}

// Operation states.
const (
	OpQueued    = "queued"
	OpClaimed   = "claimed"
	OpDone      = "done"
	OpFailed    = "failed"
	OpCancelled = "cancelled"
)

// Clip decisions.
const (
	DecisionPending  = "pending"
	DecisionApproved = "approved"
	DecisionRejected = "rejected"
)

// SubmissionStatuses are the post states a user can record, in order.
var SubmissionStatuses = []string{"Submitted", "Accepted", "Rejected"}

var ErrNotFound = errors.New("not found")

// ErrOpCancelled tells the worker to stop the operation.
var ErrOpCancelled = errors.New("operation cancelled")

func migrateMediaV4(ctx context.Context, tx *sql.Tx) error {
	return execAll(ctx, tx, []string{
		`CREATE TABLE sources (
			id INTEGER PRIMARY KEY,
			title TEXT NOT NULL,
			origin TEXT NOT NULL CHECK(origin IN ('file','url')),
			origin_ref TEXT NOT NULL,
			path TEXT NOT NULL,
			size INTEGER NOT NULL DEFAULT 0,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			width INTEGER NOT NULL DEFAULT 0,
			height INTEGER NOT NULL DEFAULT 0,
			has_audio INTEGER NOT NULL DEFAULT 0,
			rights_confirmed_at TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE jobs (
			id INTEGER PRIMARY KEY,
			title TEXT NOT NULL,
			origin TEXT NOT NULL CHECK(origin IN ('file','url')),
			origin_ref TEXT NOT NULL,
			source_id INTEGER REFERENCES sources(id),
			campaign_id TEXT REFERENCES campaign_sources(id) ON DELETE SET NULL,
			state TEXT NOT NULL,
			detail TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			clip_count INTEGER NOT NULL,
			min_s INTEGER NOT NULL,
			max_s INTEGER NOT NULL,
			rights_confirmed_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			started_at TEXT NOT NULL DEFAULT '',
			finished_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX jobs_state ON jobs(state, id)`,
		`CREATE TABLE job_steps (
			job_id INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
			step TEXT NOT NULL,
			ord INTEGER NOT NULL,
			state TEXT NOT NULL,
			detail TEXT NOT NULL DEFAULT '',
			done INTEGER NOT NULL DEFAULT 0,
			total INTEGER NOT NULL DEFAULT 0,
			running_since INTEGER NOT NULL DEFAULT 0,
			elapsed_ms INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY(job_id, step)
		)`,
		`CREATE TABLE job_log (
			id INTEGER PRIMARY KEY,
			job_id INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
			at TEXT NOT NULL,
			level TEXT NOT NULL,
			message TEXT NOT NULL
		)`,
		`CREATE INDEX job_log_job ON job_log(job_id, id)`,
		`CREATE TABLE transcripts (
			job_id INTEGER PRIMARY KEY REFERENCES jobs(id) ON DELETE CASCADE,
			model TEXT NOT NULL,
			language TEXT NOT NULL,
			segments TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE candidates (
			id INTEGER PRIMARY KEY,
			job_id INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
			ord INTEGER NOT NULL,
			start_ms INTEGER NOT NULL,
			end_ms INTEGER NOT NULL CHECK(end_ms > start_ms),
			title TEXT NOT NULL,
			hook TEXT NOT NULL,
			payoff TEXT NOT NULL,
			why TEXT NOT NULL,
			strength INTEGER NOT NULL,
			text TEXT NOT NULL,
			model TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE reviews (
			candidate_id INTEGER PRIMARY KEY REFERENCES candidates(id) ON DELETE CASCADE,
			verdict TEXT NOT NULL CHECK(verdict IN ('accept','reject')),
			reason TEXT NOT NULL,
			model TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE clips (
			id INTEGER PRIMARY KEY,
			job_id INTEGER NOT NULL REFERENCES jobs(id),
			candidate_id INTEGER NOT NULL REFERENCES candidates(id),
			path TEXT NOT NULL,
			srt_path TEXT NOT NULL,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			width INTEGER NOT NULL DEFAULT 0,
			height INTEGER NOT NULL DEFAULT 0,
			size INTEGER NOT NULL DEFAULT 0,
			captions TEXT NOT NULL CHECK(captions IN ('burned','sidecar')),
			qa TEXT NOT NULL DEFAULT '',
			qa_pass INTEGER NOT NULL DEFAULT 0,
			decision TEXT NOT NULL DEFAULT 'pending' CHECK(decision IN ('pending','approved','rejected')),
			decided_at TEXT NOT NULL DEFAULT '',
			note TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX clips_job ON clips(job_id)`,
		`CREATE TABLE submissions (
			id INTEGER PRIMARY KEY,
			clip_id INTEGER NOT NULL REFERENCES clips(id),
			campaign_id TEXT NOT NULL REFERENCES campaign_sources(id),
			post_url TEXT NOT NULL,
			status TEXT NOT NULL,
			views INTEGER CHECK(views IS NULL OR views >= 0),
			earnings_cents INTEGER CHECK(earnings_cents IS NULL OR earnings_cents >= 0),
			notes TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX submissions_clip ON submissions(clip_id)`,
		`CREATE TABLE ops (
			id INTEGER PRIMARY KEY,
			job_id INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
			step TEXT NOT NULL,
			kind TEXT NOT NULL,
			args TEXT NOT NULL,
			state TEXT NOT NULL,
			done INTEGER NOT NULL DEFAULT 0,
			total INTEGER NOT NULL DEFAULT 0,
			detail TEXT NOT NULL DEFAULT '',
			result TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			claimed_at TEXT NOT NULL DEFAULT '',
			heartbeat_ms INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX ops_state ON ops(state, id)`,
	})
}

// Source is an imported original under archive/.
type Source struct {
	ID                  int64
	Title, Origin, Ref  string
	Path                string // relative to the media root
	Size, DurationMS    int64
	Width, Height       int
	HasAudio            bool
	RightsAt, CreatedAt string
}

// Step is one pipeline stage of a job.
type Step struct {
	Name, State, Detail string
	Done, Total         int64
	RunningSince        int64 // unix ms while running
	ElapsedMS           int64
}

// Elapsed is the time spent in the step, including the current run.
func (s Step) Elapsed() time.Duration {
	ms := s.ElapsedMS
	if s.State == StepRunning && s.RunningSince > 0 {
		ms += time.Now().UnixMilli() - s.RunningSince
	}
	return time.Duration(ms) * time.Millisecond
}

// Job is one run of the pipeline over one source.
type Job struct {
	ID                        int64
	Title, Origin, Ref        string
	SourceID                  int64
	CampaignID, CampaignTitle string
	State, Detail, Error      string
	ClipCount, MinS, MaxS     int
	RightsAt                  string
	CreatedAt, StartedAt      string
	FinishedAt, UpdatedAt     string
	// RedoClipID and RedoNote are set on a job made by the Review page's Retry:
	// it hunts one replacement for that clip. Attempt counts the redo chain.
	RedoClipID int64
	RedoNote   string
	Attempt    int
	Steps      []Step
}

// Elapsed is the summed step time.
func (j Job) Elapsed() time.Duration {
	var d time.Duration
	for _, s := range j.Steps {
		d += s.Elapsed()
	}
	return d
}

// Step returns the named step.
func (j Job) Step(name string) Step {
	for _, s := range j.Steps {
		if s.Name == name {
			return s
		}
	}
	return Step{Name: name}
}

type JobInput struct {
	Title, Origin, Ref, CampaignID string
	ClipCount, MinS, MaxS          int
}

func nowMS() int64 { return time.Now().UnixMilli() }

func (s *Store) CreateJob(ctx context.Context, in JobInput) (int64, error) {
	if in.Origin != "file" && in.Origin != "url" {
		return 0, InputErrorf("Choose a file or a URL.")
	}
	if in.ClipCount < 1 || in.ClipCount > 5 {
		return 0, InputErrorf("Clip count must be 1 to 5.")
	}
	if in.MinS < 5 || in.MaxS > 180 || in.MinS >= in.MaxS {
		return 0, InputErrorf("Clip length must be between 5 and 180 seconds, shortest below longest.")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	t := now()
	res, err := tx.ExecContext(ctx, `INSERT INTO jobs(title,origin,origin_ref,campaign_id,state,clip_count,min_s,max_s,rights_confirmed_at,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`, in.Title, in.Origin, in.Ref, nullableString(in.CampaignID), JobQueued, in.ClipCount, in.MinS, in.MaxS, t, t, t)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for i, step := range Steps {
		if _, err = tx.ExecContext(ctx, `INSERT INTO job_steps(job_id,step,ord,state) VALUES(?,?,?,?)`, id, step, i, StepPending); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

const jobColumns = `j.id,j.title,j.origin,j.origin_ref,COALESCE(j.source_id,0),COALESCE(j.campaign_id,''),COALESCE(c.title,''),
	j.state,j.detail,j.error,j.clip_count,j.min_s,j.max_s,j.rights_confirmed_at,j.created_at,j.started_at,j.finished_at,j.updated_at,
	COALESCE(j.redo_clip_id,0),j.redo_note,j.attempt
	FROM jobs j LEFT JOIN campaign_sources c ON c.id=j.campaign_id`

func scanJob(row scanner) (Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.Title, &j.Origin, &j.Ref, &j.SourceID, &j.CampaignID, &j.CampaignTitle, &j.State, &j.Detail, &j.Error,
		&j.ClipCount, &j.MinS, &j.MaxS, &j.RightsAt, &j.CreatedAt, &j.StartedAt, &j.FinishedAt, &j.UpdatedAt,
		&j.RedoClipID, &j.RedoNote, &j.Attempt)
	return j, err
}

func (s *Store) loadSteps(ctx context.Context, j *Job) error {
	rows, err := s.db.QueryContext(ctx, `SELECT step,state,detail,done,total,running_since,elapsed_ms FROM job_steps WHERE job_id=? ORDER BY ord`, j.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var st Step
		if err := rows.Scan(&st.Name, &st.State, &st.Detail, &st.Done, &st.Total, &st.RunningSince, &st.ElapsedMS); err != nil {
			return err
		}
		j.Steps = append(j.Steps, st)
	}
	return rows.Err()
}

func (s *Store) JobByID(ctx context.Context, id int64) (Job, error) {
	j, err := scanJob(s.db.QueryRowContext(ctx, `SELECT `+jobColumns+` WHERE j.id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return j, ErrNotFound
	}
	if err != nil {
		return j, err
	}
	return j, s.loadSteps(ctx, &j)
}

// ListJobs returns the newest jobs first, with their steps.
func (s *Store) ListJobs(ctx context.Context, limit int) ([]Job, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+jobColumns+` ORDER BY j.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		jobs = append(jobs, j)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range jobs {
		if err := s.loadSteps(ctx, &jobs[i]); err != nil {
			return nil, err
		}
	}
	return jobs, nil
}

// NextQueuedJob returns the oldest queued job.
func (s *Store) NextQueuedJob(ctx context.Context) (Job, bool, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM jobs WHERE state=? ORDER BY id LIMIT 1`, JobQueued).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	j, err := s.JobByID(ctx, id)
	return j, err == nil, err
}

// SetJobState records a job state, its current detail and error text.
func (s *Store) SetJobState(ctx context.Context, id int64, state, detail, errMsg string) error {
	t := now()
	started, finished := "", ""
	switch state {
	case JobRunning:
		started = t
	case JobFailed, JobCancelled, JobCompleted:
		finished = t
	}
	return requireMediaRow(s.db.ExecContext(ctx, `UPDATE jobs SET state=?, detail=?, error=?, updated_at=?,
		started_at=CASE WHEN started_at='' THEN ? ELSE started_at END,
		finished_at=? WHERE id=?`, state, detail, errMsg, t, started, finished, id))
}

// SetJobDetail updates only the job's detail line.
func (s *Store) SetJobDetail(ctx context.Context, id int64, detail string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE jobs SET detail=?, updated_at=? WHERE id=?`, detail, now(), id)
	return err
}

func (s *Store) SetJobSource(ctx context.Context, jobID, sourceID int64) error {
	return requireMediaRow(s.db.ExecContext(ctx, `UPDATE jobs SET source_id=?, updated_at=? WHERE id=?`, sourceID, now(), jobID))
}

// SetStep moves a step to a new state, accumulating elapsed time when it
// leaves the running state.
func (s *Store) SetStep(ctx context.Context, jobID int64, step, state, detail string) error {
	t := nowMS()
	if state == StepRunning {
		return requireMediaRow(s.db.ExecContext(ctx, `UPDATE job_steps SET state=?, detail=?, done=0, total=0, running_since=? WHERE job_id=? AND step=?`,
			state, detail, t, jobID, step))
	}
	return requireMediaRow(s.db.ExecContext(ctx, `UPDATE job_steps SET state=?, detail=?,
		elapsed_ms=elapsed_ms + CASE WHEN running_since>0 THEN ?-running_since ELSE 0 END, running_since=0
		WHERE job_id=? AND step=?`, state, detail, t, jobID, step))
}

// SetStepProgress records real progress counts for a running step.
func (s *Store) SetStepProgress(ctx context.Context, jobID int64, step string, done, total int64, detail string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE job_steps SET done=?, total=?, detail=? WHERE job_id=? AND step=?`, done, total, detail, jobID, step)
	return err
}

// RetryJob requeues a failed, blocked or cancelled job; finished steps are kept.
func (s *Store) RetryJob(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE jobs SET state=?, error='', detail='', finished_at='', updated_at=? WHERE id=? AND state IN (?,?,?)`,
		JobQueued, now(), id, JobFailed, JobBlocked, JobCancelled)
	if err = requireMediaRow(res, err); err != nil {
		return InputErrorf("Only failed, blocked or cancelled jobs can be retried.")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE job_steps SET state=?, detail='', done=0, total=0, running_since=0 WHERE job_id=? AND state<>?`,
		StepPending, id, StepDone); err != nil {
		return err
	}
	return tx.Commit()
}

// RecoverInterrupted fails jobs left running by a previous server process and
// cancels their open operations.
func (s *Store) RecoverInterrupted(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT j.id, s.step FROM jobs j JOIN job_steps s ON s.job_id=j.id AND s.state=? WHERE j.state=?`, StepRunning, JobRunning)
	if err != nil {
		return 0, err
	}
	type pair struct {
		id   int64
		step string
	}
	var steps []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.id, &p.step); err != nil {
			rows.Close()
			return 0, err
		}
		steps = append(steps, p)
	}
	rows.Close()
	for _, p := range steps {
		if err := s.SetStep(ctx, p.id, p.step, StepFailed, "Interrupted"); err != nil {
			return 0, err
		}
	}
	res, err := s.db.ExecContext(ctx, `UPDATE jobs SET state=?, detail='', error=?, finished_at=?, updated_at=? WHERE state=?`,
		JobFailed, "Interrupted by a server restart. Retry continues from this step.", now(), now(), JobRunning)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if _, err = s.db.ExecContext(ctx, `UPDATE ops SET state=?, error='server restarted' WHERE state IN (?,?)`, OpCancelled, OpQueued, OpClaimed); err != nil {
		return 0, err
	}
	return int(n), nil
}

type LogLine struct{ At, Level, Message string }

func (s *Store) AppendLog(ctx context.Context, jobID int64, level, message string) error {
	if len(message) > 4000 {
		message = message[:4000] + "…"
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO job_log(job_id,at,level,message) VALUES(?,?,?,?)`, jobID, now(), level, message)
	return err
}

// JobLog returns the last limit lines, oldest first.
func (s *Store) JobLog(ctx context.Context, jobID int64, limit int) ([]LogLine, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT at,level,message FROM (SELECT id,at,level,message FROM job_log WHERE job_id=? ORDER BY id DESC LIMIT ?) ORDER BY id`, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LogLine
	for rows.Next() {
		var l LogLine
		if err := rows.Scan(&l.At, &l.Level, &l.Message); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) AddSource(ctx context.Context, src Source) (int64, error) {
	t := now()
	res, err := s.db.ExecContext(ctx, `INSERT INTO sources(title,origin,origin_ref,path,size,duration_ms,width,height,has_audio,rights_confirmed_at,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`, src.Title, src.Origin, src.Ref, src.Path, src.Size, src.DurationMS, src.Width, src.Height, src.HasAudio, src.RightsAt, t)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

const sourceColumns = `id,title,origin,origin_ref,path,size,duration_ms,width,height,has_audio,rights_confirmed_at,created_at FROM sources`

func scanSource(row scanner) (Source, error) {
	var src Source
	err := row.Scan(&src.ID, &src.Title, &src.Origin, &src.Ref, &src.Path, &src.Size, &src.DurationMS, &src.Width, &src.Height, &src.HasAudio, &src.RightsAt, &src.CreatedAt)
	return src, err
}

func (s *Store) SourceByID(ctx context.Context, id int64) (Source, error) {
	src, err := scanSource(s.db.QueryRowContext(ctx, `SELECT `+sourceColumns+` WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return src, ErrNotFound
	}
	return src, err
}

func (s *Store) ListSources(ctx context.Context) ([]Source, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sourceColumns+` ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Source
	for rows.Next() {
		src, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

// Segment is one transcribed stretch of speech, in source time.
type Segment struct {
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
	Text    string `json:"text"`
}

func (s *Store) SaveTranscript(ctx context.Context, jobID int64, model, language string, segments []Segment) error {
	b, err := json.Marshal(segments)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO transcripts(job_id,model,language,segments,created_at) VALUES(?,?,?,?,?)
		ON CONFLICT(job_id) DO UPDATE SET model=excluded.model, language=excluded.language, segments=excluded.segments, created_at=excluded.created_at`,
		jobID, model, language, string(b), now())
	return err
}

func (s *Store) Transcript(ctx context.Context, jobID int64) ([]Segment, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT segments FROM transcripts WHERE job_id=?`, jobID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var segs []Segment
	return segs, json.Unmarshal([]byte(raw), &segs)
}

// Candidate is a clip proposed by the hunter model.
type Candidate struct {
	ID, JobID                int64
	Ord                      int
	StartMS, EndMS           int64
	Title, Hook, Payoff, Why string
	Strength                 int
	Text, Model              string
	Attempt                  int // 1 for the first hunt; higher for a retry's pick
	// Review, filled by ListCandidates.
	Verdict, Reason, ReviewModel string
	// Scores and Parts are JSON the reviewer's scores and labelled parts were
	// saved as; empty for candidates reviewed before they existed.
	Scores, Parts string
}

// ReplaceCandidates stores a job's candidates, replacing any earlier ones
// and their reviews.
func (s *Store) ReplaceCandidates(ctx context.Context, jobID int64, cands []Candidate) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var clips int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM clips WHERE job_id=?`, jobID).Scan(&clips); err != nil {
		return err
	}
	if clips > 0 {
		return fmt.Errorf("job %d already has clips", jobID)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM candidates WHERE job_id=?`, jobID); err != nil {
		return err
	}
	t := now()
	for i, c := range cands {
		if _, err = tx.ExecContext(ctx, `INSERT INTO candidates(job_id,ord,start_ms,end_ms,title,hook,payoff,why,strength,text,model,attempt,created_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, jobID, i+1, c.StartMS, c.EndMS, c.Title, c.Hook, c.Payoff, c.Why, c.Strength, c.Text, c.Model, max(c.Attempt, 1), t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AddCandidate appends a candidate to a job's list and returns it with its
// ID and order number filled in.
func (s *Store) AddCandidate(ctx context.Context, jobID int64, c Candidate) (Candidate, error) {
	c.JobID, c.Attempt = jobID, max(c.Attempt, 1)
	err := s.db.QueryRowContext(ctx, `INSERT INTO candidates(job_id,ord,start_ms,end_ms,title,hook,payoff,why,strength,text,model,attempt,created_at)
		VALUES(?,(SELECT COALESCE(max(ord),0)+1 FROM candidates WHERE job_id=?),?,?,?,?,?,?,?,?,?,?,?) RETURNING id,ord`,
		jobID, jobID, c.StartMS, c.EndMS, c.Title, c.Hook, c.Payoff, c.Why, c.Strength, c.Text, c.Model, c.Attempt, now()).Scan(&c.ID, &c.Ord)
	return c, err
}

func (s *Store) ListCandidates(ctx context.Context, jobID int64) ([]Candidate, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.id,c.job_id,c.ord,c.start_ms,c.end_ms,c.title,c.hook,c.payoff,c.why,c.strength,c.text,c.model,c.attempt,
		COALESCE(r.verdict,''),COALESCE(r.reason,''),COALESCE(r.model,''),c.scores,c.parts
		FROM candidates c LEFT JOIN reviews r ON r.candidate_id=c.id WHERE c.job_id=? ORDER BY c.ord`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Candidate
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.ID, &c.JobID, &c.Ord, &c.StartMS, &c.EndMS, &c.Title, &c.Hook, &c.Payoff, &c.Why, &c.Strength, &c.Text, &c.Model, &c.Attempt,
			&c.Verdict, &c.Reason, &c.ReviewModel, &c.Scores, &c.Parts); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SaveReview stores a candidate's review. scores and parts are JSON (or
// empty) and are kept on the candidate.
func (s *Store) SaveReview(ctx context.Context, candidateID int64, verdict, reason, model, scores, parts string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO reviews(candidate_id,verdict,reason,model,created_at) VALUES(?,?,?,?,?)
		ON CONFLICT(candidate_id) DO UPDATE SET verdict=excluded.verdict, reason=excluded.reason, model=excluded.model, created_at=excluded.created_at`,
		candidateID, verdict, reason, model, now()); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE candidates SET scores=?, parts=? WHERE id=?`, scores, parts, candidateID); err != nil {
		return err
	}
	return tx.Commit()
}

// Clip is a rendered candidate.
type Clip struct {
	ID, JobID, CandidateID int64
	Path, SRTPath          string // relative to the media root
	DurationMS, Size       int64
	Width, Height          int
	Captions, QA           string
	QAPass                 bool
	Decision, DecidedAt    string
	Note, CreatedAt        string
	// Joined for display.
	JobTitle, CampaignID, CampaignTitle string
	Title, Hook, Text, Verdict, Reason  string
	Scores, Parts                       string // JSON, see Candidate
	StartMS, EndMS                      int64
	Attempt                             int // of the clip's job
	Submissions                         []Submission
}

func (s *Store) AddClip(ctx context.Context, c Clip) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO clips(job_id,candidate_id,path,srt_path,captions,created_at) VALUES(?,?,?,?,?,?)`,
		c.JobID, c.CandidateID, c.Path, c.SRTPath, c.Captions, now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// DeletePendingClips removes clip records of a job that no one has decided
// on, so a render retry starts clean. Files are handled by the caller.
func (s *Store) DeletePendingClips(ctx context.Context, jobID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM clips WHERE job_id=? AND decision='pending' AND id NOT IN (SELECT clip_id FROM submissions)
		AND id NOT IN (SELECT redo_clip_id FROM jobs WHERE redo_clip_id IS NOT NULL)`, jobID)
	return err
}

func (s *Store) SetClipQA(ctx context.Context, c Clip) error {
	return requireMediaRow(s.db.ExecContext(ctx, `UPDATE clips SET path=?, srt_path=?, duration_ms=?, width=?, height=?, size=?, qa=?, qa_pass=? WHERE id=?`,
		c.Path, c.SRTPath, c.DurationMS, c.Width, c.Height, c.Size, c.QA, c.QAPass, c.ID))
}

const clipColumns = `k.id,k.job_id,k.candidate_id,k.path,k.srt_path,k.duration_ms,k.size,k.width,k.height,k.captions,k.qa,k.qa_pass,
	k.decision,k.decided_at,k.note,k.created_at,j.title,COALESCE(j.campaign_id,''),COALESCE(cs.title,''),
	c.title,c.hook,c.text,COALESCE(r.verdict,''),COALESCE(r.reason,''),c.start_ms,c.end_ms,j.attempt,c.scores,c.parts
	FROM clips k JOIN jobs j ON j.id=k.job_id JOIN candidates c ON c.id=k.candidate_id
	LEFT JOIN reviews r ON r.candidate_id=c.id LEFT JOIN campaign_sources cs ON cs.id=j.campaign_id`

func scanClip(row scanner) (Clip, error) {
	var c Clip
	err := row.Scan(&c.ID, &c.JobID, &c.CandidateID, &c.Path, &c.SRTPath, &c.DurationMS, &c.Size, &c.Width, &c.Height, &c.Captions, &c.QA, &c.QAPass,
		&c.Decision, &c.DecidedAt, &c.Note, &c.CreatedAt, &c.JobTitle, &c.CampaignID, &c.CampaignTitle,
		&c.Title, &c.Hook, &c.Text, &c.Verdict, &c.Reason, &c.StartMS, &c.EndMS, &c.Attempt, &c.Scores, &c.Parts)
	return c, err
}

func (s *Store) ClipByID(ctx context.Context, id int64) (Clip, error) {
	c, err := scanClip(s.db.QueryRowContext(ctx, `SELECT `+clipColumns+` WHERE k.id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// ListClips returns clips that passed QA, optionally only those with one
// decision, newest first, with their submissions.
func (s *Store) ListClips(ctx context.Context, decision string, jobID int64) ([]Clip, error) {
	q := `SELECT ` + clipColumns + ` WHERE (?='' OR k.decision=?) AND (?=0 OR k.job_id=?) ORDER BY k.id DESC`
	rows, err := s.db.QueryContext(ctx, q, decision, decision, jobID, jobID)
	if err != nil {
		return nil, err
	}
	var out []Clip
	for rows.Next() {
		c, err := scanClip(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Submissions, err = s.submissionsFor(ctx, out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Submission is a post of an approved clip to a campaign, recorded by hand.
type Submission struct {
	ID, ClipID    int64
	CampaignID    string
	PostURL       string
	Status        string
	Views         *int64
	EarningsCents *int64
	Notes         string
	CreatedAt     string
	UpdatedAt     string
}

func validSubmissionStatus(v string) bool {
	for _, s := range SubmissionStatuses {
		if s == v {
			return true
		}
	}
	return false
}

func validPostURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || len(raw) > 2000 {
		return "", InputErrorf("The post URL must be an https:// link.")
	}
	return raw, nil
}

// AddSubmission records a post of an approved clip against its job's campaign.
func (s *Store) AddSubmission(ctx context.Context, clipID int64, postURL, status, notes string) (int64, error) {
	c, err := s.ClipByID(ctx, clipID)
	if err != nil {
		return 0, err
	}
	if c.Decision != DecisionApproved {
		return 0, InputErrorf("Approve the clip before recording a post.")
	}
	if c.CampaignID == "" {
		return 0, InputErrorf("This clip's job has no campaign.")
	}
	if postURL, err = validPostURL(postURL); err != nil {
		return 0, err
	}
	if !validSubmissionStatus(status) {
		return 0, InputErrorf("Unknown post status.")
	}
	if err = validateNotes(notes); err != nil {
		return 0, err
	}
	t := now()
	res, err := s.db.ExecContext(ctx, `INSERT INTO submissions(clip_id,campaign_id,post_url,status,notes,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`,
		clipID, c.CampaignID, postURL, status, notes, t, t)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateSubmission(ctx context.Context, id int64, status string, views, earnings *int64, notes string) error {
	if !validSubmissionStatus(status) {
		return InputErrorf("Unknown post status.")
	}
	if views != nil && *views < 0 {
		return InputErrorf("Views cannot be negative.")
	}
	if err := validateNotes(notes); err != nil {
		return err
	}
	return requireMediaRow(s.db.ExecContext(ctx, `UPDATE submissions SET status=?, views=?, earnings_cents=?, notes=?, updated_at=? WHERE id=?`,
		status, views, earnings, notes, now(), id))
}

func (s *Store) submissionsFor(ctx context.Context, clipID int64) ([]Submission, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,clip_id,campaign_id,post_url,status,views,earnings_cents,notes,created_at,updated_at
		FROM submissions WHERE clip_id=? ORDER BY id`, clipID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Submission
	for rows.Next() {
		var sub Submission
		var views, earn sql.NullInt64
		if err := rows.Scan(&sub.ID, &sub.ClipID, &sub.CampaignID, &sub.PostURL, &sub.Status, &views, &earn, &sub.Notes, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, err
		}
		if views.Valid {
			sub.Views = &views.Int64
		}
		if earn.Valid {
			sub.EarningsCents = &earn.Int64
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

// CampaignWork counts a campaign's clips and recorded posts.
type CampaignWork struct {
	Jobs, Clips, Approved, Posts int
	Views, EarningsCents         int64
}

func (s *Store) CampaignWork(ctx context.Context) (map[string]CampaignWork, error) {
	out := map[string]CampaignWork{}
	rows, err := s.db.QueryContext(ctx, `SELECT j.campaign_id, count(DISTINCT j.id),
		count(k.id), COALESCE(sum(k.decision='approved'),0) FROM jobs j LEFT JOIN clips k ON k.job_id=j.id AND k.qa_pass=1
		WHERE j.campaign_id IS NOT NULL GROUP BY j.campaign_id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		var w CampaignWork
		if err := rows.Scan(&id, &w.Jobs, &w.Clips, &w.Approved); err != nil {
			rows.Close()
			return nil, err
		}
		out[id] = w
	}
	rows.Close()
	rows, err = s.db.QueryContext(ctx, `SELECT campaign_id, count(*), COALESCE(sum(views),0), COALESCE(sum(earnings_cents),0) FROM submissions GROUP BY campaign_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var posts int
		var views, earn int64
		if err := rows.Scan(&id, &posts, &views, &earn); err != nil {
			return nil, err
		}
		w := out[id]
		w.Posts, w.Views, w.EarningsCents = posts, views, earn
		out[id] = w
	}
	return out, rows.Err()
}

// Op is one media operation for the worker.
type Op struct {
	ID, JobID          int64
	Step, Kind, State  string
	Args, Result       json.RawMessage
	Done, Total        int64
	Detail, Error      string
	CreatedAt, Claimed string
	HeartbeatMS        int64
}

func (s *Store) CreateOp(ctx context.Context, jobID int64, step, kind string, args any) (int64, error) {
	b, err := json.Marshal(args)
	if err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO ops(job_id,step,kind,args,state,created_at) VALUES(?,?,?,?,?,?)`, jobID, step, kind, string(b), OpQueued, now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func scanOp(row scanner) (Op, error) {
	var o Op
	var args, result string
	err := row.Scan(&o.ID, &o.JobID, &o.Step, &o.Kind, &o.State, &args, &result, &o.Done, &o.Total, &o.Detail, &o.Error, &o.CreatedAt, &o.Claimed, &o.HeartbeatMS)
	o.Args, o.Result = json.RawMessage(args), json.RawMessage(result)
	return o, err
}

const opColumns = `id,job_id,step,kind,state,args,result,done,total,detail,error,created_at,claimed_at,heartbeat_ms FROM ops`

func (s *Store) OpByID(ctx context.Context, id int64) (Op, error) {
	o, err := scanOp(s.db.QueryRowContext(ctx, `SELECT `+opColumns+` WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return o, ErrNotFound
	}
	return o, err
}

// ClaimOp hands the oldest queued operation to the worker.
func (s *Store) ClaimOp(ctx context.Context) (Op, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Op{}, false, err
	}
	defer tx.Rollback()
	o, err := scanOp(tx.QueryRowContext(ctx, `SELECT `+opColumns+` WHERE state=? ORDER BY id LIMIT 1`, OpQueued))
	if errors.Is(err, sql.ErrNoRows) {
		return Op{}, false, nil
	}
	if err != nil {
		return Op{}, false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE ops SET state=?, claimed_at=?, heartbeat_ms=? WHERE id=?`, OpClaimed, now(), nowMS(), o.ID); err != nil {
		return Op{}, false, err
	}
	o.State = OpClaimed
	return o, true, tx.Commit()
}

// OpProgress records worker progress and a heartbeat, and copies the counts
// to the job step. It returns ErrOpCancelled when the operation is no longer
// claimed.
func (s *Store) OpProgress(ctx context.Context, id, done, total int64, detail string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE ops SET done=?, total=?, detail=?, heartbeat_ms=? WHERE id=? AND state=?`, done, total, detail, nowMS(), id, OpClaimed)
	if err = requireMediaRow(res, err); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrOpCancelled
		}
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE job_steps SET done=?, total=?, detail=? WHERE (job_id, step) = (SELECT job_id, step FROM ops WHERE id=?)`,
		done, total, detail, id)
	return err
}

// FinishOp records the worker's result. A cancelled operation stays cancelled.
func (s *Store) FinishOp(ctx context.Context, id int64, ok bool, result json.RawMessage, errMsg string) error {
	state := OpDone
	if !ok {
		state = OpFailed
	}
	if len(result) == 0 {
		result = json.RawMessage("null")
	}
	res, err := s.db.ExecContext(ctx, `UPDATE ops SET state=?, result=?, error=?, heartbeat_ms=? WHERE id=? AND state=?`, state, string(result), errMsg, nowMS(), id, OpClaimed)
	if err = requireMediaRow(res, err); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrOpCancelled
		}
		return err
	}
	return nil
}

func (s *Store) CancelOp(ctx context.Context, id int64, reason string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE ops SET state=?, error=? WHERE id=? AND state IN (?,?)`, OpCancelled, reason, id, OpQueued, OpClaimed)
	return err
}

// FailStaleOps fails claimed operations whose last heartbeat is older than
// the cutoff.
func (s *Store) FailStaleOps(ctx context.Context, cutoff time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE ops SET state=?, error=? WHERE state=? AND heartbeat_ms < ?`,
		OpFailed, "The worker stopped responding.", OpClaimed, cutoff.UnixMilli())
	return err
}

func requireMediaRow(result sql.Result, err error) error {
	if err = requireRow(result, err); errors.Is(err, ErrCampaignNotFound) {
		return ErrNotFound
	}
	return err
}

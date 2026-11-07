package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Lesson kinds: a clip the user liked, or a problem to avoid.
const (
	LessonGood = "good"
	LessonFix  = "fix"
)

// LessonNoteMax is the longest lesson note, in characters.
const LessonNoteMax = 300

// fixNeedsNote is the message when a note is needed for a rejection that
// teaches the model or retries the clip.
const fixNeedsNote = "Add a note saying what to fix."

func migrateLessonsV5(ctx context.Context, tx *sql.Tx) error {
	return execAll(ctx, tx, []string{
		`CREATE TABLE lessons (
			id INTEGER PRIMARY KEY,
			clip_id INTEGER UNIQUE REFERENCES clips(id) ON DELETE SET NULL,
			kind TEXT NOT NULL CHECK(kind IN ('good','fix')),
			title TEXT NOT NULL DEFAULT '',
			note TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`ALTER TABLE jobs ADD COLUMN redo_clip_id INTEGER REFERENCES clips(id)`,
		`ALTER TABLE jobs ADD COLUMN redo_note TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE jobs ADD COLUMN attempt INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE candidates ADD COLUMN attempt INTEGER NOT NULL DEFAULT 1`,
	})
}

// migrateScoresV6 adds the reviewer's scores and labelled parts (JSON text)
// to candidates; older rows keep them empty.
func migrateScoresV6(ctx context.Context, tx *sql.Tx) error {
	return execAll(ctx, tx, []string{
		`ALTER TABLE candidates ADD COLUMN scores TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE candidates ADD COLUMN parts TEXT NOT NULL DEFAULT ''`,
	})
}

// Lesson is what the user taught the models from one reviewed clip.
type Lesson struct {
	ID, ClipID int64
	Kind       string
	Title      string
	Note       string
	CreatedAt  string
}

// ListLessons returns the newest lessons first.
func (s *Store) ListLessons(ctx context.Context, limit int) ([]Lesson, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,COALESCE(clip_id,0),kind,title,note,created_at FROM lessons ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Lesson
	for rows.Next() {
		var l Lesson
		if err := rows.Scan(&l.ID, &l.ClipID, &l.Kind, &l.Title, &l.Note, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) DeleteLesson(ctx context.Context, id int64) error {
	return requireMediaRow(s.db.ExecContext(ctx, `DELETE FROM lessons WHERE id=?`, id))
}

// cutRunes shortens s to at most n characters.
func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// DecideClip records the user's decision on a clip that passed QA. With
// feedback on, an approved clip becomes a "good" lesson and a rejected one a
// "fix" lesson (which needs a note); otherwise the clip has no lesson. There
// is at most one lesson per clip.
func (s *Store) DecideClip(ctx context.Context, id int64, decision, note string, feedback bool) error {
	if decision != DecisionApproved && decision != DecisionRejected && decision != DecisionPending {
		return InputErrorf("Unknown decision.")
	}
	if err := validateNotes(note); err != nil {
		return err
	}
	lessonNote := cutRunes(strings.TrimSpace(note), LessonNoteMax)
	if feedback && decision == DecisionRejected && lessonNote == "" {
		return InputErrorf(fixNeedsNote)
	}
	decided := now()
	if decision == DecisionPending {
		decided = ""
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var title string
	err = tx.QueryRowContext(ctx, `SELECT c.title FROM clips k JOIN candidates c ON c.id=k.candidate_id WHERE k.id=? AND k.qa_pass=1`, id).Scan(&title)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err = requireMediaRow(tx.ExecContext(ctx, `UPDATE clips SET decision=?, decided_at=?, note=? WHERE id=? AND qa_pass=1`, decision, decided, note, id)); err != nil {
		return err
	}
	// Replacing the lesson (rather than updating it) keeps ids in time order.
	if _, err = tx.ExecContext(ctx, `DELETE FROM lessons WHERE clip_id=?`, id); err != nil {
		return err
	}
	if feedback && decision != DecisionPending {
		kind := LessonGood
		if decision == DecisionRejected {
			kind = LessonFix
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO lessons(clip_id,kind,title,note,created_at) VALUES(?,?,?,?,?)`, id, kind, title, lessonNote, now()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CreateRedoJob queues a job that hunts one replacement for a rejected clip,
// guided by the user's note. It reuses the clip's source and transcript, so
// the import and transcribe steps start out done.
func (s *Store) CreateRedoJob(ctx context.Context, clipID int64, note string) (int64, error) {
	note = strings.TrimSpace(note)
	if note == "" {
		return 0, InputErrorf(fixNeedsNote)
	}
	if err := validateNotes(note); err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var (
		old                              Job
		sourceID                         sql.NullInt64
		campaignID                       sql.NullString
		title                            string
		originRef, origin, rightsConfirm string
	)
	err = tx.QueryRowContext(ctx, `SELECT j.id,j.source_id,j.campaign_id,j.min_s,j.max_s,j.attempt,j.origin,j.origin_ref,j.rights_confirmed_at,c.title
		FROM clips k JOIN jobs j ON j.id=k.job_id JOIN candidates c ON c.id=k.candidate_id WHERE k.id=?`, clipID).
		Scan(&old.ID, &sourceID, &campaignID, &old.MinS, &old.MaxS, &old.Attempt, &origin, &originRef, &rightsConfirm, &title)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	attempt := old.Attempt + 1
	t := now()
	res, err := tx.ExecContext(ctx, `INSERT INTO jobs(title,origin,origin_ref,source_id,campaign_id,state,clip_count,min_s,max_s,rights_confirmed_at,
		redo_clip_id,redo_note,attempt,created_at,updated_at) VALUES(?,?,?,?,?,?,1,?,?,?,?,?,?,?,?)`,
		fmt.Sprintf("%s — attempt %d", cutRunes(title, 120), attempt), origin, originRef, sourceID, campaignID, JobQueued, old.MinS, old.MaxS, rightsConfirm,
		clipID, note, attempt, t, t)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	copied, err := tx.ExecContext(ctx, `INSERT INTO transcripts(job_id,model,language,segments,created_at)
		SELECT ?,model,language,segments,created_at FROM transcripts WHERE job_id=?`, id, old.ID)
	if err != nil {
		return 0, err
	}
	if n, _ := copied.RowsAffected(); n == 0 {
		return 0, InputErrorf("The original job has no transcript to reuse.")
	}
	for i, step := range Steps {
		state, detail := StepPending, ""
		if step == "import" || step == "transcribe" {
			state, detail = StepDone, fmt.Sprintf("Reused from job %d", old.ID)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO job_steps(job_id,step,ord,state,detail) VALUES(?,?,?,?,?)`, id, step, i, state, detail); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

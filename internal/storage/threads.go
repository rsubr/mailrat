package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// ThreadSummary is a thread as shown in the inbox: aggregated from its messages.
type ThreadSummary struct {
	ThreadID       string
	AccountID      string
	Subject        string
	Snippet        string // from the most recent message
	Participants   string // comma-separated distinct from_addr values
	MessageCount   int
	UnreadCount    int
	LastDate       int64
	HasAttachments bool
	IsStarred      bool
	LabelIDs       []string // distinct label IDs across all messages in thread
}

// pageSQL selects one page of thread keys from the candidate query cand
// (columns thread_id, account_id), newest thread first. Only the page's
// threads are aggregated by threadSummarySQL. The thread date is a cheap
// index seek on idx_messages_thread.
func pageSQL(cand string) string {
	return `WITH cand AS (` + cand + `),
	page AS (
		SELECT c.thread_id, c.account_id,
			(SELECT MAX(x.internal_date) FROM messages x
			 WHERE x.account_id = c.account_id AND x.thread_id = c.thread_id) AS last_date
		FROM cand c
		ORDER BY last_date DESC, c.thread_id, c.account_id
		LIMIT ? OFFSET ?
	)
	`
}

const threadSummarySQL = `
	SELECT
		m.thread_id,
		m.account_id,
		COUNT(*)                                                   AS message_count,
		SUM(m.is_unread)                                           AS unread_count,
		MAX(m.internal_date)                                       AS last_date,
		MAX(m.has_attachments)                                     AS has_attachments,
		GROUP_CONCAT(DISTINCT m.from_addr)                         AS participants,
		(SELECT subject FROM messages m2
		 WHERE m2.thread_id = m.thread_id AND m2.account_id = m.account_id
		 ORDER BY m2.internal_date DESC LIMIT 1)                   AS subject,
		(SELECT snippet  FROM messages m2
		 WHERE m2.thread_id = m.thread_id AND m2.account_id = m.account_id
		 ORDER BY m2.internal_date DESC LIMIT 1)                   AS snippet,
		(SELECT GROUP_CONCAT(DISTINCT ml.label_id)
		 FROM message_labels ml
		 JOIN messages m2 ON m2.id = ml.message_id AND m2.account_id = ml.account_id
		 WHERE m2.thread_id = m.thread_id AND m2.account_id = m.account_id) AS label_ids,
		(SELECT COUNT(*) FROM message_labels ml
		 JOIN messages m2 ON m2.id = ml.message_id AND m2.account_id = ml.account_id
		 WHERE m2.thread_id = m.thread_id AND m2.account_id = m.account_id
		   AND ml.label_id = 'STARRED')                            AS is_starred
	FROM page p
	JOIN messages m ON m.account_id = p.account_id AND m.thread_id = p.thread_id
	GROUP BY m.thread_id, m.account_id
	ORDER BY last_date DESC, m.thread_id, m.account_id
`

// queryThreadPage runs one page of thread summaries for the candidate query.
func (s *Store) queryThreadPage(cand string, args []any, limit, offset int) ([]ThreadSummary, error) {
	rows, err := s.db.Query(pageSQL(cand)+threadSummarySQL, append(args, limit, offset)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanThreadSummaries(rows)
}

// labelCandidates returns the candidate query for threads with a message
// carrying labelID, optionally scoped to one account (accountID != "").
func labelCandidates(accountID, labelID string, hideTrashed bool) (string, []any) {
	q := `SELECT DISTINCT m2.thread_id, m2.account_id
		FROM message_labels ml
		JOIN messages m2 ON m2.id = ml.message_id AND m2.account_id = ml.account_id
		WHERE ml.label_id = ?`
	args := []any{labelID}
	if accountID != "" {
		q += ` AND ml.account_id = ?`
		args = append(args, accountID)
	}
	if hideTrashed {
		q += notTrashed(labelID)
	}
	return q, args
}

func scanThreadSummaries(rows *sql.Rows) ([]ThreadSummary, error) {
	var out []ThreadSummary
	for rows.Next() {
		var t ThreadSummary
		var hasAttachments, isStarred int
		var labelIDs sql.NullString
		if err := rows.Scan(
			&t.ThreadID, &t.AccountID,
			&t.MessageCount, &t.UnreadCount, &t.LastDate, &hasAttachments,
			&t.Participants, &t.Subject, &t.Snippet,
			&labelIDs, &isStarred,
		); err != nil {
			return nil, err
		}
		t.HasAttachments = hasAttachments == 1
		t.IsStarred = isStarred > 0
		if labelIDs.Valid && labelIDs.String != "" {
			t.LabelIDs = strings.Split(labelIDs.String, ",")
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListAllThreadSummaries returns inbox threads across all accounts, newest first.
func (s *Store) ListAllThreadSummaries(limit, offset int) ([]ThreadSummary, error) {
	cand, args := labelCandidates("", "INBOX", false)
	return s.queryThreadPage(cand, args, limit, offset)
}

// LocalSearchThreadSummaries searches threads locally across all accounts.
// Every whitespace-delimited term must match (as a word prefix) in some
// message of the thread: subject, body, from/to/cc/bcc, or an attachment
// filename. Results are newest first.
func (s *Store) LocalSearchThreadSummaries(rawQuery string, limit int) ([]ThreadSummary, error) {
	terms := splitSearchTerms(rawQuery)
	if len(terms) == 0 {
		return nil, nil
	}
	var parts []string
	var args []any
	for _, term := range terms {
		parts = append(parts, `SELECT thread_id, account_id FROM (
			SELECT m.thread_id, m.account_id FROM messages_fts
			JOIN messages m ON m.rowid = messages_fts.rowid
			WHERE messages_fts MATCH ?
			UNION
			SELECT m.thread_id, m.account_id FROM attachments a
			JOIN messages m ON m.id = a.message_id AND m.account_id = a.account_id
			WHERE instr(lower(a.name), ?) > 0)`)
		args = append(args, ftsPhrase(term), strings.ToLower(term))
	}
	return s.queryThreadPage(strings.Join(parts, " INTERSECT "), args, limit, 0)
}

// ftsPhrase turns a user term into a quoted FTS5 prefix phrase so punctuation
// and operators (AND, -, ", :) are matched literally instead of parsed.
func ftsPhrase(term string) string {
	return `"` + strings.ReplaceAll(term, `"`, `""`) + `"*`
}

func splitSearchTerms(query string) []string {
	var terms []string
	for _, t := range strings.Fields(query) {
		if strings.IndexFunc(t, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0 {
			terms = append(terms, t)
		}
	}
	return terms
}

// ListThreadSummaries returns inbox threads for a single account, newest first.
func (s *Store) ListThreadSummaries(accountID string, limit, offset int) ([]ThreadSummary, error) {
	cand, args := labelCandidates(accountID, "INBOX", false)
	return s.queryThreadPage(cand, args, limit, offset)
}

// ListAllThreadSummariesByLabel returns threads carrying a given label across all accounts.
func (s *Store) ListAllThreadSummariesByLabel(labelID string, limit, offset int) ([]ThreadSummary, error) {
	cand, args := labelCandidates("", labelID, true)
	return s.queryThreadPage(cand, args, limit, offset)
}

// notTrashed hides trashed messages from every label view except Trash itself
// (a trashed message keeps labels such as SENT, so it would otherwise stay listed).
func notTrashed(labelID string) string {
	if labelID == "TRASH" {
		return ""
	}
	return ` AND NOT EXISTS (SELECT 1 FROM message_labels t WHERE t.message_id = m2.id AND t.account_id = m2.account_id AND t.label_id = 'TRASH')`
}

// ListThreadSummariesByLabel returns threads carrying a given label for one account.
func (s *Store) ListThreadSummariesByLabel(accountID, labelID string, limit, offset int) ([]ThreadSummary, error) {
	cand, args := labelCandidates(accountID, labelID, true)
	return s.queryThreadPage(cand, args, limit, offset)
}

type Thread struct {
	ID              string
	AccountID       string
	LastMessageDate int64
}

const threadUpsertSQL = `
	INSERT INTO threads (id, account_id, last_message_date)
	VALUES (?, ?, ?)
	ON CONFLICT(id, account_id) DO UPDATE SET
		last_message_date = CASE
			WHEN excluded.last_message_date > threads.last_message_date
			THEN excluded.last_message_date
			ELSE threads.last_message_date
		END
`

func (s *Store) UpsertThread(t Thread) error {
	_, err := s.db.Exec(threadUpsertSQL, t.ID, t.AccountID, t.LastMessageDate)
	return err
}

func (s *Store) GetThread(accountID, threadID string) (Thread, error) {
	var t Thread
	err := s.db.QueryRow(`
		SELECT id, account_id, last_message_date FROM threads WHERE id = ? AND account_id = ?
	`, threadID, accountID).Scan(&t.ID, &t.AccountID, &t.LastMessageDate)
	if errors.Is(err, sql.ErrNoRows) {
		return Thread{}, fmt.Errorf("thread %q not found", threadID)
	}
	return t, err
}

func (s *Store) ListThreads(accountID string, limit int) ([]Thread, error) {
	rows, err := s.db.Query(`
		SELECT id, account_id, last_message_date FROM threads
		WHERE account_id = ?
		ORDER BY last_message_date DESC
		LIMIT ?
	`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Thread
	for rows.Next() {
		var t Thread
		if err := rows.Scan(&t.ID, &t.AccountID, &t.LastMessageDate); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

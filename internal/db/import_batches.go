package db

import (
	"strings"
	"time"
)

// ImportBatch is one viewing-history upload pending review/application.
type ImportBatch struct {
	ID              int64
	UserID          int64
	Source          string
	Filename        string
	Status          string // pending | applied | discarded
	Entries         int
	SeriesMatched   int
	UnmatchedSeries []string
	CreatedAt       string
	AppliedAt       string
	// Computed counts:
	TotalChanges    int
	SelectedChanges int
	AppliedChanges  int
}

// ImportChange is a single proposed watched-date change within a batch.
type ImportChange struct {
	ID           int64
	BatchID      int64
	Type         string // episode | movie
	TargetID     int64  // show id or movie id
	Title        string // WatchLog display name
	Season       int
	Episode      int
	NetflixTitle string
	CurrentDate  string
	NewDate      string
	Selected     bool
	Applied      bool
}

// CreateImportBatch inserts a new batch and returns its id.
func (db *DB) CreateImportBatch(userID int64, source, filename string, entries, seriesMatched int, unmatched []string) (int64, error) {
	res, err := db.conn.Exec(
		`INSERT INTO import_batches (user_id, source, filename, status, entries, series_matched, unmatched_series, created_at)
		 VALUES (?, ?, ?, 'pending', ?, ?, ?, ?)`,
		userID, source, filename, entries, seriesMatched, strings.Join(unmatched, "\n"), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// AddImportChanges bulk-inserts the proposed changes for a batch.
func (db *DB) AddImportChanges(batchID int64, changes []ImportChange) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(
		`INSERT INTO import_changes (batch_id, type, target_id, title, season, episode, netflix_title, current_date, new_date, selected, applied)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 0)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, c := range changes {
		if _, err := stmt.Exec(batchID, c.Type, c.TargetID, c.Title, c.Season, c.Episode, c.NetflixTitle, c.CurrentDate, c.NewDate); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func scanBatchCounts(db *DB, b *ImportBatch) {
	db.conn.QueryRow(`SELECT
		COUNT(*),
		COALESCE(SUM(selected), 0),
		COALESCE(SUM(applied), 0)
		FROM import_changes WHERE batch_id = ?`, b.ID).Scan(&b.TotalChanges, &b.SelectedChanges, &b.AppliedChanges)
}

// GetImportBatchForUser returns a batch only if it belongs to the user.
func (db *DB) GetImportBatchForUser(id, userID int64) (ImportBatch, error) {
	var b ImportBatch
	var unmatched, filename, appliedAt *string
	err := db.conn.QueryRow(
		`SELECT id, user_id, source, filename, status, entries, series_matched, unmatched_series, created_at, applied_at
		 FROM import_batches WHERE id = ? AND user_id = ?`, id, userID).
		Scan(&b.ID, &b.UserID, &b.Source, &filename, &b.Status, &b.Entries, &b.SeriesMatched, &unmatched, &b.CreatedAt, &appliedAt)
	if err != nil {
		return b, err
	}
	if filename != nil {
		b.Filename = *filename
	}
	if appliedAt != nil {
		b.AppliedAt = *appliedAt
	}
	if unmatched != nil && *unmatched != "" {
		b.UnmatchedSeries = strings.Split(*unmatched, "\n")
	}
	scanBatchCounts(db, &b)
	return b, nil
}

// ListImportBatches returns the user's batches, most recent first.
func (db *DB) ListImportBatches(userID int64) ([]ImportBatch, error) {
	rows, err := db.conn.Query(
		`SELECT id, source, filename, status, entries, series_matched, created_at, applied_at
		 FROM import_batches WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ImportBatch
	for rows.Next() {
		var b ImportBatch
		var filename, appliedAt *string
		b.UserID = userID
		if err := rows.Scan(&b.ID, &b.Source, &filename, &b.Status, &b.Entries, &b.SeriesMatched, &b.CreatedAt, &appliedAt); err != nil {
			return nil, err
		}
		if filename != nil {
			b.Filename = *filename
		}
		if appliedAt != nil {
			b.AppliedAt = *appliedAt
		}
		scanBatchCounts(db, &b)
		out = append(out, b)
	}
	return out, rows.Err()
}

// ListImportChanges returns the changes of a batch, paginated (limit<=0 = all).
func (db *DB) ListImportChanges(batchID int64, limit, offset int) ([]ImportChange, error) {
	q := `SELECT id, batch_id, type, target_id, title, season, episode, netflix_title, current_date, new_date, selected, applied
		 FROM import_changes WHERE batch_id = ? ORDER BY id ASC`
	args := []any{batchID}
	if limit > 0 {
		q += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}
	rows, err := db.conn.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ImportChange
	for rows.Next() {
		var c ImportChange
		var sel, app int
		if err := rows.Scan(&c.ID, &c.BatchID, &c.Type, &c.TargetID, &c.Title, &c.Season, &c.Episode, &c.NetflixTitle, &c.CurrentDate, &c.NewDate, &sel, &app); err != nil {
			return nil, err
		}
		c.Selected = sel == 1
		c.Applied = app == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetImportChange returns a single change scoped to its batch.
func (db *DB) GetImportChange(batchID, changeID int64) (ImportChange, error) {
	var c ImportChange
	var sel, app int
	err := db.conn.QueryRow(
		`SELECT id, batch_id, type, target_id, title, season, episode, netflix_title, current_date, new_date, selected, applied
		 FROM import_changes WHERE id = ? AND batch_id = ?`, changeID, batchID).
		Scan(&c.ID, &c.BatchID, &c.Type, &c.TargetID, &c.Title, &c.Season, &c.Episode, &c.NetflixTitle, &c.CurrentDate, &c.NewDate, &sel, &app)
	c.Selected = sel == 1
	c.Applied = app == 1
	return c, err
}

// SetImportChangeSelected toggles a change's selected flag (scoped to batch).
func (db *DB) SetImportChangeSelected(batchID, changeID int64, selected bool) error {
	v := 0
	if selected {
		v = 1
	}
	_, err := db.conn.Exec(`UPDATE import_changes SET selected = ? WHERE id = ? AND batch_id = ?`, v, changeID, batchID)
	return err
}

// UpdateImportChangeDate sets a change's new_date (scoped to batch).
func (db *DB) UpdateImportChangeDate(batchID, changeID int64, newDate string) error {
	_, err := db.conn.Exec(`UPDATE import_changes SET new_date = ? WHERE id = ? AND batch_id = ?`, newDate, changeID, batchID)
	return err
}

// MarkImportChangeApplied flags a change as applied.
func (db *DB) MarkImportChangeApplied(batchID, changeID int64) error {
	_, err := db.conn.Exec(`UPDATE import_changes SET applied = 1 WHERE id = ? AND batch_id = ?`, changeID, batchID)
	return err
}

// SetImportBatchStatus updates a batch status (and applied_at when applied).
func (db *DB) SetImportBatchStatus(id int64, status string) error {
	if status == "applied" {
		_, err := db.conn.Exec(`UPDATE import_batches SET status = ?, applied_at = ? WHERE id = ?`,
			status, time.Now().UTC().Format(time.RFC3339), id)
		return err
	}
	_, err := db.conn.Exec(`UPDATE import_batches SET status = ? WHERE id = ?`, status, id)
	return err
}

// DeleteImportBatch removes a batch and its changes (FK cascade).
func (db *DB) DeleteImportBatch(id, userID int64) error {
	_, err := db.conn.Exec(`DELETE FROM import_batches WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

package handlers

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/mdaguete/watchlog/internal/db"
	"github.com/mdaguete/watchlog/internal/i18n"
	"github.com/mdaguete/watchlog/internal/importer"
)

const historyPageSize = 100

// PageHistoryImport renders the upload page plus the user's recent batches.
func (h *Handler) PageHistoryImport(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 {
		return
	}
	lang := h.getLang(r, userID)
	batches, _ := h.DB.ListImportBatches(userID)
	h.Templates.ExecuteTemplate(w, "history_import.html", map[string]any{
		"Lang":    lang,
		"Batches": batches,
	})
}

// HandleHistoryAnalyze parses the CSV, persists a batch with its proposed
// changes, and redirects to the batch review page.
func (h *Handler) HandleHistoryAnalyze(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 {
		return
	}
	lang := h.getLang(r, userID)

	if err := r.ParseMultipartForm(64 << 20); err != nil {
		h.renderHistoryError(w, lang, userID, "history.error.upload")
		return
	}
	source := r.FormValue("source")
	if source == "" {
		source = "netflix"
	}
	file, header, err := r.FormFile("csvfile")
	if err != nil {
		h.renderHistoryError(w, lang, userID, "history.error.no_file")
		return
	}
	defer file.Close()

	analysis, err := importer.AnalyzeNetflix(h.DB, userID, file)
	if err != nil {
		log.Printf("history import: analyze error: %v", err)
		h.renderHistoryError(w, lang, userID, "history.error.parse")
		return
	}

	batchID, err := h.DB.CreateImportBatch(userID, source, header.Filename, analysis.Entries, analysis.SeriesMatched, analysis.UnmatchedSeries)
	if err != nil {
		log.Printf("history import: create batch error: %v", err)
		h.renderHistoryError(w, lang, userID, "history.error.parse")
		return
	}
	changes := make([]db.ImportChange, 0, len(analysis.Changes))
	for _, c := range analysis.Changes {
		changes = append(changes, db.ImportChange{
			Type: c.Type, TargetID: c.ID, Title: c.Title,
			Season: c.Season, Episode: c.Episode, NetflixTitle: c.NetflixTitle,
			CurrentDate: c.CurrentDate, NewDate: c.NewDate,
		})
	}
	if err := h.DB.AddImportChanges(batchID, changes); err != nil {
		log.Printf("history import: add changes error: %v", err)
	}
	// Persist unmatched entries for later TMDB reconciliation.
	unmatched := make([]db.UnmatchedEntry, 0, len(analysis.UnmatchedEntries))
	for _, e := range analysis.UnmatchedEntries {
		kind := "series"
		if e.IsMovie {
			kind = "movie"
		}
		unmatched = append(unmatched, db.UnmatchedEntry{
			Kind: kind, NetflixName: e.Series, Season: e.Season,
			NetflixEp: e.EpTitle, WatchedDate: e.Date.Format("2006-01-02"),
		})
	}
	if err := h.DB.AddImportUnmatched(batchID, unmatched); err != nil {
		log.Printf("history import: add unmatched error: %v", err)
	}
	log.Printf("ACTION: history import batch %d created (%d changes, %d unmatched)", batchID, len(changes), len(unmatched))
	http.Redirect(w, r, "/import/history/"+strconv.FormatInt(batchID, 10), http.StatusSeeOther)
}

// PageHistoryBatch renders a paginated, editable review of a batch.
func (h *Handler) PageHistoryBatch(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 {
		return
	}
	lang := h.getLang(r, userID)
	batchID, ok := h.parsePathID(w, r, "id")
	if !ok {
		return
	}
	batch, err := h.DB.GetImportBatchForUser(batchID, userID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * historyPageSize
	changes, _ := h.DB.ListImportChanges(batchID, historyPageSize, offset)
	totalPages := (batch.TotalChanges + historyPageSize - 1) / historyPageSize
	unmatchedGroups, _ := h.DB.ListUnmatchedGroups(batchID)

	h.Templates.ExecuteTemplate(w, "history_batch.html", map[string]any{
		"Lang":            lang,
		"Batch":           batch,
		"Changes":         changes,
		"UnmatchedGroups": unmatchedGroups,
		"TMDBEnabled":     h.TMDB != nil && h.TMDB.Enabled(),
		"Page":            page,
		"TotalPages":      totalPages,
		"HasPrev":         page > 1,
		"HasNext":         page < totalPages,
		"PrevPage":        page - 1,
		"NextPage":        page + 1,
	})
}

// HandleHistoryToggle flips a change's selected flag (HTMX).
func (h *Handler) HandleHistoryToggle(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 {
		return
	}
	batchID, ok := h.parsePathID(w, r, "id")
	if !ok {
		return
	}
	if _, err := h.DB.GetImportBatchForUser(batchID, userID); err != nil {
		http.NotFound(w, r)
		return
	}
	changeID, ok := h.parsePathID(w, r, "cid")
	if !ok {
		return
	}
	selected := r.FormValue("selected") == "true" || r.FormValue("selected") == "on"
	if err := h.DB.SetImportChangeSelected(batchID, changeID, selected); err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleHistoryEditDate updates a change's new_date and returns the row (HTMX).
func (h *Handler) HandleHistoryEditDate(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 {
		return
	}
	lang := h.getLang(r, userID)
	batchID, ok := h.parsePathID(w, r, "id")
	if !ok {
		return
	}
	if _, err := h.DB.GetImportBatchForUser(batchID, userID); err != nil {
		http.NotFound(w, r)
		return
	}
	changeID, ok := h.parsePathID(w, r, "cid")
	if !ok {
		return
	}
	newDate := r.FormValue("new_date")
	if _, err := time.Parse("2006-01-02", newDate); err != nil {
		http.Error(w, "invalid date", http.StatusBadRequest)
		return
	}
	if err := h.DB.UpdateImportChangeDate(batchID, changeID, newDate); err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	c, err := h.DB.GetImportChange(batchID, changeID)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	h.Templates.ExecuteTemplate(w, "history_change_row", map[string]any{"Lang": lang, "Change": c, "Batch": db.ImportBatch{ID: batchID}})
}

// HandleHistoryApply backs up the DB and applies the selected, unapplied changes.
func (h *Handler) HandleHistoryApply(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 {
		return
	}
	batchID, ok := h.parsePathID(w, r, "id")
	if !ok {
		return
	}
	batch, err := h.DB.GetImportBatchForUser(batchID, userID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	changes, err := h.DB.ListImportChanges(batchID, 0, 0)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	// Backup once before any write.
	bkp, err := h.DB.Backup("history-import")
	if err != nil {
		log.Printf("history import: backup error: %v", err)
		lang := h.getLang(r, userID)
		h.renderHistoryError(w, lang, userID, "history.error.backup")
		return
	}

	applied, failed := 0, 0
	for _, c := range changes {
		if !c.Selected || c.Applied {
			continue
		}
		nc := importer.NetflixChange{Type: c.Type, ID: c.TargetID, Season: c.Season, Episode: c.Episode, NewDate: c.NewDate}
		if err := importer.ApplyNetflixChange(h.DB, userID, nc); err != nil {
			log.Printf("history import: apply change %d error: %v", c.ID, err)
			failed++
			continue
		}
		h.DB.MarkImportChangeApplied(batchID, c.ID)
		applied++
	}
	h.DB.SetImportBatchStatus(batchID, "applied")
	h.DB.SyncWatchStatsFromDB(userID)
	log.Printf("ACTION: history import batch %d applied %d changes (%d failed), backup=%s", batch.ID, applied, failed, bkp)
	http.Redirect(w, r, "/import/history/"+strconv.FormatInt(batchID, 10), http.StatusSeeOther)
}

// HandleHistoryDelete discards a batch and its staged changes.
func (h *Handler) HandleHistoryDelete(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 {
		return
	}
	batchID, ok := h.parsePathID(w, r, "id")
	if !ok {
		return
	}
	if err := h.DB.DeleteImportBatch(batchID, userID); err != nil {
		log.Printf("history import: delete batch error: %v", err)
	}
	http.Redirect(w, r, "/import/history", http.StatusSeeOther)
}

func (h *Handler) renderHistoryError(w http.ResponseWriter, lang string, userID int64, key string) {
	batches, _ := h.DB.ListImportBatches(userID)
	h.Templates.ExecuteTemplate(w, "history_import.html", map[string]any{
		"Lang":    lang,
		"Batches": batches,
		"Error":   i18n.T(lang, key),
	})
}

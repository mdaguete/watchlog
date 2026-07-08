package handlers

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/mdaguete/watchlog/internal/auth"
	"github.com/mdaguete/watchlog/internal/models"
)

func TestPageHistoryImport_RendersUpload(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/import/history", token, nil)
	rec := httptest.NewRecorder()
	h.PageHistoryImport(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `action="/import/history"`) || !strings.Contains(body, `name="csvfile"`) {
		t.Errorf("upload form not rendered")
	}
}

func formPost(path, token, body string) *http.Request {
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	return req
}

func TestHistoryImport_FullFlow(t *testing.T) {
	h, userID, token := newTestHandler(t)

	// Seed a show with Spanish name + a watched episode.
	showID, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "Money Heist"})
	h.DB.UpdateShowTMDBNames(showID, "La casa de papel", "Money Heist")
	h.DB.MarkEpisodeWatched(userID, showID, 4, 1)

	// 1) Upload → analyze → batch created + redirect.
	csv := "Title,Date\n\"La casa de papel: Parte 4: Game Over\",\"4/3/20\"\n"
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("csvfile", "NetflixViewingHistory.csv")
	fw.Write([]byte(csv))
	mw.WriteField("source", "netflix")
	mw.Close()
	req := authedRequest("POST", "/import/history", token, buf.Bytes())
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.HandleHistoryAnalyze(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("analyze expected redirect, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/import/history/") {
		t.Fatalf("unexpected redirect: %q", loc)
	}
	batchID, _ := strconv.ParseInt(strings.TrimPrefix(loc, "/import/history/"), 10, 64)

	// Verify the batch + change persisted.
	batch, err := h.DB.GetImportBatchForUser(batchID, userID)
	if err != nil {
		t.Fatalf("batch not found: %v", err)
	}
	if batch.TotalChanges != 1 {
		t.Fatalf("expected 1 change, got %d", batch.TotalChanges)
	}
	changes, _ := h.DB.ListImportChanges(batchID, 0, 0)
	changeID := changes[0].ID
	if changes[0].NewDate != "2020-04-03" {
		t.Fatalf("unexpected change date %q", changes[0].NewDate)
	}

	// 2) Batch review page renders the change.
	greq := authedRequest("GET", loc, token, nil)
	greq.SetPathValue("id", strconv.FormatInt(batchID, 10))
	grec := httptest.NewRecorder()
	h.PageHistoryBatch(grec, greq)
	if grec.Code != http.StatusOK || !strings.Contains(grec.Body.String(), "La casa de papel") {
		t.Fatalf("batch page did not render change (status %d)", grec.Code)
	}

	// 3) Edit the date via HTMX endpoint.
	dreq := formPost(loc+"/change/"+strconv.FormatInt(changeID, 10)+"/date", token, url.Values{"new_date": {"2020-04-05"}}.Encode())
	dreq.SetPathValue("id", strconv.FormatInt(batchID, 10))
	dreq.SetPathValue("cid", strconv.FormatInt(changeID, 10))
	drec := httptest.NewRecorder()
	h.HandleHistoryEditDate(drec, dreq)
	if drec.Code != http.StatusOK {
		t.Fatalf("edit date status %d", drec.Code)
	}
	if c, _ := h.DB.GetImportChange(batchID, changeID); c.NewDate != "2020-04-05" {
		t.Fatalf("date not updated, got %q", c.NewDate)
	}

	// 4) Toggle off then on.
	treq := formPost(loc+"/change/"+strconv.FormatInt(changeID, 10)+"/toggle", token, "selected=false")
	treq.SetPathValue("id", strconv.FormatInt(batchID, 10))
	treq.SetPathValue("cid", strconv.FormatInt(changeID, 10))
	trec := httptest.NewRecorder()
	h.HandleHistoryToggle(trec, treq)
	if c, _ := h.DB.GetImportChange(batchID, changeID); c.Selected {
		t.Fatalf("toggle off failed")
	}
	// Re-select for apply.
	treq2 := formPost(loc+"/change/"+strconv.FormatInt(changeID, 10)+"/toggle", token, "selected=true")
	treq2.SetPathValue("id", strconv.FormatInt(batchID, 10))
	treq2.SetPathValue("cid", strconv.FormatInt(changeID, 10))
	h.HandleHistoryToggle(httptest.NewRecorder(), treq2)

	// 5) Apply.
	areq := formPost(loc+"/apply", token, "")
	areq.SetPathValue("id", strconv.FormatInt(batchID, 10))
	arec := httptest.NewRecorder()
	h.HandleHistoryApply(arec, areq)
	if arec.Code != http.StatusSeeOther {
		t.Fatalf("apply expected redirect, got %d", arec.Code)
	}
	// Episode date updated to the edited value.
	if got := h.DB.GetEpisodeWatchedAt(userID, showID, 4, 1); !strings.HasPrefix(got, "2020-04-05") {
		t.Errorf("watched_at not updated, got %q", got)
	}
	b2, _ := h.DB.GetImportBatchForUser(batchID, userID)
	if b2.Status != "applied" || b2.AppliedChanges != 1 {
		t.Errorf("batch not marked applied: %+v", b2)
	}
}

func TestHistoryReconcile_TMDBDisabledGuards(t *testing.T) {
	h, userID, token := newTestHandler(t) // TMDB is nil in tests
	batchID, _ := h.DB.CreateImportBatch(userID, "netflix", "f.csv", 1, 0, nil)

	sreq := authedRequest("GET", "/import/history/1/tmdb?name=Arcane&kind=series", token, nil)
	sreq.SetPathValue("id", strconv.FormatInt(batchID, 10))
	srec := httptest.NewRecorder()
	h.HandleHistoryTMDBSearch(srec, sreq)
	if srec.Code != http.StatusServiceUnavailable {
		t.Errorf("search: expected 503 when TMDB disabled, got %d", srec.Code)
	}

	rreq := formPost("/import/history/1/resolve", token, url.Values{"name": {"Arcane"}, "kind": {"series"}, "tmdb_id": {"123"}}.Encode())
	rreq.SetPathValue("id", strconv.FormatInt(batchID, 10))
	rrec := httptest.NewRecorder()
	h.HandleHistoryResolve(rrec, rreq)
	if rrec.Code != http.StatusServiceUnavailable {
		t.Errorf("resolve: expected 503 when TMDB disabled, got %d", rrec.Code)
	}
}

func TestHistoryAnalyze_PersistsUnmatched(t *testing.T) {
	h, userID, token := newTestHandler(t)
	// No shows seeded -> everything is unmatched.
	csv := "Title,Date\n\"Arcane: Temporada 1: Bienvenidos\",\"1/1/22\"\n\"Some Movie\",\"5/1/20\"\n"
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("csvfile", "n.csv")
	fw.Write([]byte(csv))
	mw.WriteField("source", "netflix")
	mw.Close()
	req := authedRequest("POST", "/import/history", token, buf.Bytes())
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.HandleHistoryAnalyze(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rec.Code)
	}
	batchID, _ := strconv.ParseInt(strings.TrimPrefix(rec.Header().Get("Location"), "/import/history/"), 10, 64)
	groups, _ := h.DB.ListUnmatchedGroups(batchID)
	if len(groups) != 2 {
		t.Fatalf("expected 2 unmatched groups (series + movie), got %d: %+v", len(groups), groups)
	}
	_ = userID
}

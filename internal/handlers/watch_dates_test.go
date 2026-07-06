package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mdaguete/watchlog/internal/auth"
	"github.com/mdaguete/watchlog/internal/models"
)

func jsonReq(method, path, token, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	}
	return req
}

func TestAPISetEpisodeDate(t *testing.T) {
	h, uid, token := newTestHandler(t)
	sid, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	h.DB.MarkEpisodeWatched(uid, sid, 1, 2)

	req := jsonReq("POST", "/api/shows/1/episodes/date", token, `{"season":1,"episode":2,"datetime":"2021-05-04T20:30"}`)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APISetEpisodeDate(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	eps, _ := h.DB.GetEpisodesByShow(uid, sid)
	var found bool
	for _, e := range eps {
		if e.SeasonNumber == 1 && e.EpisodeNumber == 2 {
			found = true
			if e.WatchedAt.Year() != 2021 || e.WatchedAt.Month() != 5 || e.WatchedAt.Day() != 4 || e.WatchedAt.Hour() != 20 || e.WatchedAt.Minute() != 30 {
				t.Errorf("watched_at = %v, want 2021-05-04 20:30", e.WatchedAt)
			}
		}
	}
	if !found {
		t.Error("episode not found after update")
	}
}

func TestAPISetEpisodeDate_NotWatched(t *testing.T) {
	h, _, token := newTestHandler(t)
	h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	req := jsonReq("POST", "/api/shows/1/episodes/date", token, `{"season":5,"episode":9,"datetime":"2021-05-04T20:30"}`)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APISetEpisodeDate(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for unwatched episode", w.Code)
	}
}

func TestAPISetEpisodeDate_InvalidDatetime(t *testing.T) {
	h, uid, token := newTestHandler(t)
	sid, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	h.DB.MarkEpisodeWatched(uid, sid, 1, 1)
	req := jsonReq("POST", "/api/shows/1/episodes/date", token, `{"season":1,"episode":1,"datetime":"nope"}`)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APISetEpisodeDate(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for invalid datetime", w.Code)
	}
}

func TestAPISetEpisodeDate_Unauthenticated(t *testing.T) {
	h, _, _ := newTestHandler(t)
	req := jsonReq("POST", "/api/shows/1/episodes/date", "", `{"season":1,"episode":1,"datetime":"2021-05-04T20:30"}`)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APISetEpisodeDate(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestAPISetMovieDate(t *testing.T) {
	h, uid, token := newTestHandler(t)
	mid, _ := h.DB.UpsertMovie(models.Movie{ExternalID: "m1", Name: "M"})
	h.DB.AddMovieToLibrary(uid, mid)
	h.DB.MarkMovieWatched(uid, mid, time.Now())

	req := jsonReq("POST", "/api/movies/1/date", token, `{"datetime":"2020-01-09T17:22"}`)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APISetMovieDate(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	at, ok := h.DB.GetMovieWatchedAt(uid, mid)
	if !ok || at.Year() != 2020 || at.Month() != 1 || at.Day() != 9 || at.Hour() != 17 || at.Minute() != 22 {
		t.Errorf("watched_at = %v (ok=%v), want 2020-01-09 17:22", at, ok)
	}
}

func TestAPISetMovieDate_NotWatched(t *testing.T) {
	h, _, token := newTestHandler(t)
	h.DB.UpsertMovie(models.Movie{ExternalID: "m1", Name: "M"})
	req := jsonReq("POST", "/api/movies/1/date", token, `{"datetime":"2020-01-09T17:22"}`)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APISetMovieDate(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for unwatched movie", w.Code)
	}
}

package db

import (
	"testing"
	"time"

	"github.com/mdaguete/watchlog/internal/models"
)

func TestListAllUsers_Counts(t *testing.T) {
	database := providersTestDB(t)
	uid, _ := database.CreateUser("u", "x")

	showID, _ := database.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	database.FollowShow(uid, showID)
	database.MarkEpisodeWatched(uid, showID, 1, 1)
	database.MarkEpisodeWatched(uid, showID, 1, 2)

	movieID, _ := database.UpsertMovie(models.Movie{ExternalID: "m1", Name: "M"})
	database.MarkMovieWatched(uid, movieID, time.Now())

	users, err := database.ListAllUsers()
	if err != nil {
		t.Fatal(err)
	}
	var got *CLIUser
	for i := range users {
		if users[i].ID == uid {
			got = &users[i]
		}
	}
	if got == nil {
		t.Fatal("user not found")
	}
	if got.Shows != 1 {
		t.Errorf("Shows = %d, want 1", got.Shows)
	}
	if got.Movies != 1 {
		t.Errorf("Movies = %d, want 1", got.Movies)
	}
	if got.Episodes != 2 {
		t.Errorf("Episodes = %d, want 2", got.Episodes)
	}
}

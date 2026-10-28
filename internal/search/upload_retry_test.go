package search

import (
	"context"
	"errors"
	"ravenseed/internal/database"
	"ravenseed/internal/models"
	"strings"
	"sync"
	"testing"
)

func TestReuploadReusesDownloadWithNewFlagsAndTMDb(t *testing.T) {
	f := newUploadFixture(t)
	f.mu.Lock()
	f.progress, f.state, f.files, f.exitCode = 1, "stalledUP", true, 1
	f.mu.Unlock()
	id := f.queue(t, "retry")
	f.s.ProcessUpload(context.Background(), id)
	first, _ := f.s.Store.Upload(id)
	if first.Status != "failed" {
		t.Fatal(first.Status)
	}
	tmdb := 604
	input := models.RetryUploadRequest{Args: "--anon --modq", TMDbID: &tmdb}
	var wg sync.WaitGroup
	outcomes := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.s.RetryUpload(id, input); outcomes <- err }()
	}
	wg.Wait()
	close(outcomes)
	successes := 0
	for err := range outcomes {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("accepted %d concurrent retries", successes)
	}
	f.mu.Lock()
	f.exitCode = 0
	f.mu.Unlock()
	f.s.ProcessUpload(context.Background(), id)
	f.s.ProcessUpload(context.Background(), id)
	retried, _ := f.s.Store.Upload(id)
	if retried.Status != "finished" || retried.Attempt != 1 || len(retried.PreviousAttempts) != 1 || retried.PreviousAttempts[0].Status != "failed" || retried.PreviousAttempts[0].Media.TMDbID != 603 {
		t.Fatalf("retry history lost: %+v", retried)
	}
	f.mu.Lock()
	calls, adds, args, session := f.calls, f.adds, f.args, f.session
	f.mu.Unlock()
	if calls != 2 || adds != 1 || session != id+"_retry_1" || !strings.Contains(args, "--anon --modq") || !strings.Contains(args, "--tmdb movie/604") {
		t.Fatalf("bad retry: calls=%d adds=%d args=%s session=%s", calls, adds, args, session)
	}
	if _, err := f.s.RetryUpload(id, input); err == nil {
		t.Fatal("stale retry accepted")
	}
	other := f.queue(t, "duplicate")
	f.s.ProcessUpload(context.Background(), other)
	if _, err := f.s.RetryUpload(other, models.RetryUploadRequest{Args: "--anon"}); err != nil {
		t.Fatal(err)
	}
	f.s.ProcessUpload(context.Background(), other)
	f.mu.Lock()
	calls = f.calls
	f.mu.Unlock()
	if calls != 2 {
		t.Fatal("retry stole another job's reservation")
	}
}

func TestReuploadValidationAndStalePoll(t *testing.T) {
	f := newUploadFixture(t)
	id := f.queue(t, "validation")
	if _, err := f.s.RetryUpload(id, models.RetryUploadRequest{}); err == nil {
		t.Fatal("active retry accepted")
	}
	stale, _ := f.s.Store.Upload(id)
	if err := f.s.CancelUpload(id); err != nil {
		t.Fatal(err)
	}
	invalid := -1
	for _, input := range []models.RetryUploadRequest{{Args: "--trackers OTHER"}, {Args: "--tmdb movie/1"}, {TMDbID: &invalid}, {Attempt: 1}} {
		if _, err := f.s.RetryUpload(id, input); err == nil {
			t.Fatal("invalid retry accepted")
		}
	}
	job, err := f.s.RetryUpload(id, models.RetryUploadRequest{Args: "--modq"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.s.Store.SaveWaitingUpload(stale); !errors.Is(err, database.ErrUploadChanged) {
		t.Fatalf("stale poll not reported: %v", err)
	}
	after, _ := f.s.Store.Upload(id)
	if after.Attempt != job.Attempt || after.Args != "--modq" {
		t.Fatal("old poll overwrote retry")
	}
}

func TestUploadUsesResolvedTMDbFromIMDbSearch(t *testing.T) {
	for _, mediaType := range []string{"movie", "tv"} {
		t.Run(mediaType, func(t *testing.T) {
			f := newUploadFixture(t)
			f.mu.Lock()
			f.progress, f.state, f.files = 1, "stalledUP", true
			f.mu.Unlock()
			j := models.Job{ID: "resolved", Request: models.SearchRequest{TargetIndexerID: 3, Media: models.Media{Type: mediaType, IMDbID: "tt0133093"}}, Media: &models.MediaSnapshot{Media: models.Media{Type: mediaType, IMDbID: "tt0133093", TMDbID: 603}}}
			out, err := f.s.QueueUpload(context.Background(), j, models.Result{ID: "r", IndexerID: 1, DownloadURL: "https://tracker.example/file"}, models.AddTorrentRequest{InstanceID: 7})
			if err != nil {
				t.Fatal(err)
			}
			f.s.ProcessUpload(context.Background(), out.UploadJobID)
			f.mu.Lock()
			args := f.args
			f.mu.Unlock()
			if !strings.Contains(args, "--tmdb "+mediaType+"/603") {
				t.Fatal("resolved ID not forwarded", args)
			}
		})
	}
}

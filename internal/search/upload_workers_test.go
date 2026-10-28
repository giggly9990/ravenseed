package search

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func waitUntil(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition timed out")
}

func TestUploadWorkersKeepPollingAndBoundConcurrency(t *testing.T) {
	for _, workers := range []int{1, 2} {
		t.Run(fmt.Sprint(workers), func(t *testing.T) {
			f := newUploadFixture(t, workers)
			release := make(chan struct{})
			defer close(release)
			f.mu.Lock()
			f.holdExecute = release
			f.uniqueHashes = true
			f.progress = 1
			f.state = "stalledUP"
			f.files = true
			f.mu.Unlock()
			var ids []string
			for i := 0; i < workers+1; i++ {
				ids = append(ids, f.queue(t, fmt.Sprint(i)))
			}
			f.s.StartUploads()
			f.s.StartUploads()
			waiting := ids[len(ids)-1]
			waitUntil(t, func() bool {
				j, _ := f.s.Store.Upload(waiting)
				return j.Progress == 1 && (strings.Contains(j.Message, "worker") || strings.Contains(j.Message, "queued"))
			})
			f.mu.Lock()
			calls, maximum := f.calls, f.maximum
			f.mu.Unlock()
			if calls != workers || maximum != workers {
				t.Fatalf("calls=%d max=%d workers=%d", calls, maximum, workers)
			}
			if err := f.s.CancelUpload(waiting); err != nil {
				t.Fatal(err)
			}
			f.s.Close()
			for _, id := range ids[:workers] {
				j, _ := f.s.Store.Upload(id)
				if j.Status != "needs_review" {
					t.Fatalf("interrupted execution was not quarantined: %s", j.Status)
				}
			}
			j, _ := f.s.Store.Upload(waiting)
			if j.Status != "cancelled" {
				t.Fatal("cancellation overwritten")
			}
		})
	}
}

func TestCancellationDuringSlowPoll(t *testing.T) {
	f := newUploadFixture(t)
	release := make(chan struct{})
	f.mu.Lock()
	f.holdPoll = release
	f.polled = make(chan struct{}, 1)
	f.progress = 1
	f.state = "stalledUP"
	f.files = true
	f.mu.Unlock()
	id := f.queue(t, "slow")
	done := make(chan struct{})
	go func() { defer close(done); f.s.ProcessUpload(context.Background(), id) }()
	select {
	case <-f.polled:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("poll did not start")
	}
	cancelled := make(chan error, 1)
	go func() { cancelled <- f.s.CancelUpload(id) }()
	select {
	case err := <-cancelled:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(time.Second):
		t.Error("network poll blocked cancellation")
	}
	close(release)
	<-done
	j, _ := f.s.Store.Upload(id)
	f.mu.Lock()
	calls := f.calls
	f.mu.Unlock()
	if j.Status != "cancelled" || calls != 0 {
		t.Fatal("cancelled job was submitted or overwritten")
	}
}

func TestUploadWorkerWakesAtSubmissionInterval(t *testing.T) {
	f := newUploadFixture(t, 2)
	f.s.Config.UploadSubmissionInterval = 150 * time.Millisecond
	release := make(chan struct{})
	defer close(release)
	f.mu.Lock()
	f.holdExecute = release
	f.uniqueHashes, f.progress, f.state, f.files = true, 1, "stalledUP", true
	f.mu.Unlock()
	f.queue(t, "first")
	f.queue(t, "second")
	f.s.StartUploads()
	waitUntil(t, func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.calls == 2
	})
	f.mu.Lock()
	spacing := f.callTimes[1].Sub(f.callTimes[0])
	f.mu.Unlock()
	if spacing < 140*time.Millisecond {
		t.Fatalf("submissions were only %s apart", spacing)
	}
}

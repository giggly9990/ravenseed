package search

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"ravenseed/internal/database"
	"ravenseed/internal/models"
	"ravenseed/internal/qui"
	"ravenseed/internal/uploads"
	"slices"
	"strings"
	"time"
)

func (s *Service) UploadSettings() (models.UploadSettings, error) {
	v, err := s.Store.UploadSettings()
	v.HasToken = v.Token != ""
	v.Token = ""
	return v, err
}

func (s *Service) ConfigureUploads(ctx context.Context, v models.UploadSettings, save bool) error {
	v, err := s.configureUploads(v, save)
	if err != nil || save {
		return err
	}
	c, err := uploads.New(v.URL, v.Token)
	if err != nil {
		return err
	}
	return upstream(c.Test(ctx))
}

func (s *Service) configureUploads(v models.UploadSettings, save bool) (models.UploadSettings, error) {
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	old, err := s.Store.UploadSettings()
	if err != nil {
		return v, internal(err)
	}
	if v.ClearToken && v.Token != "" {
		return v, errors.New("cannot set and clear a token together")
	}
	if !v.ClearToken && v.Token == "" {
		if strings.TrimRight(strings.TrimSpace(v.URL), "/") != old.URL && old.Token != "" {
			return v, errors.New("enter a new token when changing the Upload-Assistant URL")
		}
		v.Token = old.Token
	}
	if v.ClearToken {
		v.Token = ""
	}
	if v.FlagPresets == nil {
		v.FlagPresets = old.FlagPresets
		v.DefaultFlagPresetID = old.DefaultFlagPresetID
	}
	v, err = uploads.Validate(v)
	if err != nil {
		return v, err
	}
	if save {
		err = internal(s.Store.SaveUploadSettings(v))
	}
	return v, err
}

func (s *Service) QueueUpload(ctx context.Context, j models.Job, r models.Result, input models.AddTorrentRequest) (models.AddTorrentResponse, error) {
	if err := qui.CheckDownloadLink(r); err != nil {
		return models.AddTorrentResponse{}, err
	}
	s.uploadMu.Lock()
	v, err := s.Store.UploadSettings()
	if err != nil {
		s.uploadMu.Unlock()
		return models.AddTorrentResponse{}, internal(err)
	}
	if !v.Enabled {
		s.uploadMu.Unlock()
		return models.AddTorrentResponse{}, errors.New("enable Upload-Assistant in Settings first")
	}
	presetID := v.DefaultFlagPresetID
	if input.UploadPresetID != nil {
		presetID = *input.UploadPresetID
	}
	preset := models.UploadFlagPreset{}
	if presetID != "" {
		for _, p := range v.FlagPresets {
			if p.ID == presetID {
				preset = p
			}
		}
		if preset.ID == "" {
			s.uploadMu.Unlock()
			return models.AddTorrentResponse{}, errors.New("selected UA preset no longer exists; select another preset")
		}
	}
	target := j.Request.TargetIndexerID
	code := ""
	for _, t := range v.Trackers {
		if t.IndexerID == target {
			code = t.Code
		}
	}
	if target == 0 || code == "" || target == r.IndexerID {
		s.uploadMu.Unlock()
		return models.AddTorrentResponse{}, errors.New("choose a mapped search target different from the source tracker")
	}
	paths := []models.UploadPath{}
	for _, p := range v.Paths {
		if p.InstanceID == input.InstanceID {
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		s.uploadMu.Unlock()
		return models.AddTorrentResponse{}, errors.New("configure a download path mapping for this qui instance first")
	}
	var id [16]byte
	if _, err = rand.Read(id[:]); err != nil {
		s.uploadMu.Unlock()
		return models.AddTorrentResponse{}, internal(err)
	}
	now := time.Now().UTC()
	idString := "upload_" + hex.EncodeToString(id[:])
	s.mu.RLock()
	settings := s.settings
	s.mu.RUnlock()
	job := models.UploadJob{ID: idString, SearchID: j.ID, ResultID: r.ID, Title: r.Title, InstanceID: input.InstanceID, TargetIndexerID: target, Tracker: code, Tag: "ravenseed-" + hex.EncodeToString(id[:]), Status: "adding", Message: "Adding torrent to qui", Media: j.Request.Media, QuiURL: settings.QuiURL, AssistantURL: v.URL, Paths: paths, CreatedAt: now, UpdatedAt: now}
	job.FlagPresetName, job.Args = preset.Name, preset.Args
	if j.Media != nil && j.Media.TMDbID > 0 && j.Media.Type == j.Request.Media.Type {
		job.Media = j.Media.Media
	}
	if err = s.Store.CreateUpload(job); err != nil {
		s.uploadMu.Unlock()
		return models.AddTorrentResponse{}, errors.New("could not create upload job; check Upload queue for an existing submission")
	}
	s.uploadMu.Unlock()
	c, err := qui.New(settings.QuiURL, settings.APIKey, s.Config.SearchTimeout)
	if input.Paused == nil {
		paused := false
		input.Paused = &paused
	}
	if input.Tags != "" {
		input.Tags += ","
	}
	input.Tags += job.Tag
	var out models.AddTorrentResponse
	if err == nil {
		out, err = c.AddTorrent(ctx, r, input)
	}
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	if err != nil {
		job.Status, job.Message = "needs_review", "qui did not confirm the add. Check the destination before adding again."
		_ = s.Store.SaveUpload(job)
		if errors.Is(err, context.DeadlineExceeded) {
			return out, TimeoutHint{"Check Upload queue before retrying; qui did not confirm the add.", err}
		}
		return out, upstream(errors.New("qui add was not confirmed; see Upload queue before retrying"))
	}
	job.Status, job.Message = "downloading", "Waiting for the complete download"
	if err = s.Store.SaveUpload(job); err != nil {
		return out, internal(fmt.Errorf("torrent added but queue update failed: %w", err))
	}
	out.UploadJobID = job.ID
	s.wakeUploads()
	return out, nil
}

// wakeUploads makes the upload worker poll now.
func (s *Service) wakeUploads() {
	select {
	case s.uploadWake <- struct{}{}:
	default:
	}
}

var errMonitorDropped = errors.New("the monitor no longer scans this torrent's qui instance")

// QueueExistingUpload reuses a complete source torrent already in qui. It never
// adds a second copy; the normal upload worker still verifies every file.
// The qui lookup runs without the upload lock; the job's unique key and a
// guarded candidate update keep concurrent requests from queueing twice.
func (s *Service) QueueExistingUpload(ctx context.Context, candidateID string, input models.MonitorUploadRequest) (models.UploadJob, error) {
	v, err := s.Store.MonitorCandidate(candidateID)
	if err != nil {
		return models.UploadJob{}, internal(err)
	}
	m, err := s.Store.Monitor(v.MonitorID)
	if err != nil {
		return models.UploadJob{}, internal(err)
	}
	manualReview := v.Status == "uncertain" && v.Policy != nil && v.Policy.Classification == models.PolicyPossibleTrump
	if (v.Status != "candidate" && !manualReview) || v.UploadJobID != "" || v.LastCheckedAt == nil || time.Since(*v.LastCheckedAt) > 48*time.Hour {
		return models.UploadJob{}, errors.New("candidate needs a recent successful target check and must not be queued already")
	}
	if input.InstanceID != v.InstanceID {
		return models.UploadJob{}, errors.New("choose the source torrent's qui instance")
	}
	if !monitorCovers(m, v.InstanceID) {
		return models.UploadJob{}, errMonitorDropped
	}
	if input.TMDbID < 0 {
		return models.UploadJob{}, errors.New("TMDb ID must be positive")
	}
	settings, err := s.Store.UploadSettings()
	if err != nil {
		return models.UploadJob{}, internal(err)
	}
	if !settings.Enabled {
		return models.UploadJob{}, errors.New("enable Upload-Assistant in Settings first")
	}
	code := ""
	for _, t := range settings.Trackers {
		if t.IndexerID == m.TargetIndexerID {
			code = t.Code
		}
	}
	if code == "" {
		return models.UploadJob{}, errors.New("map the target indexer to an Upload-Assistant tracker first")
	}
	paths := []models.UploadPath{}
	for _, p := range settings.Paths {
		if p.InstanceID == v.InstanceID {
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		return models.UploadJob{}, errors.New("configure a path mapping for the source qui instance first")
	}
	presetID := settings.DefaultFlagPresetID
	if input.UploadPresetID != nil {
		presetID = *input.UploadPresetID
	}
	preset := models.UploadFlagPreset{}
	if presetID != "" {
		for _, p := range settings.FlagPresets {
			if p.ID == presetID {
				preset = p
				break
			}
		}
		if preset.ID == "" {
			return models.UploadJob{}, errors.New("selected UA preset no longer exists")
		}
	}
	s.mu.RLock()
	connection := s.settings
	s.mu.RUnlock()
	if connection.QuiURL != m.QuiURL {
		return models.UploadJob{}, errors.New("qui connection changed; recreate this monitor")
	}
	c, err := qui.New(connection.QuiURL, connection.APIKey, s.Config.SearchTimeout)
	if err != nil {
		return models.UploadJob{}, err
	}
	item, err := c.TorrentByHash(ctx, v.InstanceID, v.Hash)
	if err != nil || trackerHost(item.Tracker) != m.SourceTrackerHost {
		return models.UploadJob{}, errors.New("source torrent is unavailable or its tracker changed")
	}
	if item.Progress < 1 || item.AmountLeft != 0 {
		return models.UploadJob{}, errors.New("source torrent is not complete yet")
	}
	now := time.Now().UTC()
	media := uploadMedia(v, input)
	j := models.UploadJob{ID: "upload_" + randomID(), SearchID: m.ID, ResultID: v.Hash, Title: v.Name, InstanceID: v.InstanceID, TargetIndexerID: m.TargetIndexerID, Tracker: code, Hash: v.Hash, Status: "downloading", Message: "Checking existing torrent in qui", Media: media, QuiURL: connection.QuiURL, AssistantURL: settings.URL, Paths: paths, CreatedAt: now, UpdatedAt: now, FlagPresetName: preset.Name, Args: preset.Args}
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	// The monitor's instances may have changed during the qui lookup; edits
	// save under the upload lock, so none can land before the job exists.
	if m, err = s.Store.Monitor(m.ID); err != nil {
		return models.UploadJob{}, internal(err)
	}
	if !monitorCovers(m, v.InstanceID) {
		return models.UploadJob{}, errMonitorDropped
	}
	if err = s.Store.CreateUpload(j); err != nil {
		return models.UploadJob{}, errors.New("this torrent and target already have an upload job; check Upload queue")
	}
	// Only the upload job ID changes: a check finishing meanwhile keeps its
	// results, and keeps this ID when it saves.
	if err = s.Store.SetCandidateUploadJob(v.ID, j.ID); err != nil {
		slog.Error("could not link upload job to candidate", "candidate", v.ID, "job", j.ID, "error", err)
		return j, errors.New("upload queued, but candidate update failed; check Upload queue")
	}
	s.wakeUploads()
	return j, nil
}

func (s *Service) CancelUpload(id string) error {
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	j, err := s.Store.Upload(id)
	if err != nil {
		return internal(err)
	}
	if j.Status != "downloading" {
		return errors.New("only waiting jobs can be cancelled here; check Upload-Assistant for active runs")
	}
	j.Status, j.Message = "cancelled", "Upload cancelled. The torrent remains in qui."
	return internal(s.Store.SaveUpload(j))
}

func (s *Service) RetryUpload(id string, input models.RetryUploadRequest) (models.UploadJob, error) {
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	j, err := s.Store.Upload(id)
	if err != nil {
		return j, internal(err)
	}
	if !slices.Contains([]string{"failed", "needs_review", "finished", "cancelled"}, j.Status) || j.Attempt != input.Attempt {
		return j, errors.New("job changed or is still active; refresh the queue before reuploading")
	}
	v, err := s.Store.UploadSettings()
	if err != nil {
		return j, internal(err)
	}
	if !v.Enabled {
		return j, errors.New("enable Upload-Assistant in Settings first")
	}
	args, err := uploads.NormalizeArgs(input.Args)
	if err != nil {
		return j, err
	}
	if input.TMDbID != nil && (*input.TMDbID < 0 || *input.TMDbID > 2147483647 || (j.Media.Type != "movie" && j.Media.Type != "tv")) {
		return j, errors.New("use a valid numeric TMDb ID for a movie or TV search")
	}
	j.PreviousAttempts = append(j.PreviousAttempts, models.UploadAttempt{Attempt: j.Attempt, Args: j.Args, Media: j.Media, Status: j.Status, Message: j.Message, TrackerResult: j.TrackerResult, UpdatedAt: j.UpdatedAt})
	j.Attempt++
	j.TrackerResult = nil
	j.Args, j.FlagPresetName = args, ""
	if input.TMDbID != nil {
		j.Media.TMDbID = *input.TMDbID
	}
	j.Status, j.Message = "downloading", "Reupload requested; checking the existing download"
	j.NextAttemptAt, j.RateLimitRetries = nil, 0
	j.UpdatedAt = time.Now().UTC()
	if err := s.Store.SaveUpload(j); err != nil {
		return j, internal(err)
	}
	s.wakeUploads()
	return j, nil
}

func (s *Service) StartUploads() {
	s.uploadStarted.Do(func() {
		s.jobMu.Lock()
		defer s.jobMu.Unlock()
		if s.ctx.Err() != nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			for {
				if s.ctx.Err() != nil {
					return
				}
				jobs, err := s.Store.PendingUploads()
				if err == nil {
					for _, j := range jobs {
						if s.ctx.Err() != nil {
							return
						}
						job, client, ready := s.prepareUpload(s.ctx, j.ID)
						if ready {
							s.wg.Add(1)
							go func() { defer s.wg.Done(); s.executeUpload(s.ctx, job, client) }()
						}
					}
				} else if s.ctx.Err() == nil {
					slog.Error("could not poll upload queue", "error", err)
				}
				wait := 30 * time.Second
				s.uploadMu.Lock()
				if !s.uploadDue.IsZero() {
					if until := time.Until(s.uploadDue); until < wait {
						wait = until
					}
					s.uploadDue = time.Time{}
				}
				s.uploadMu.Unlock()
				if wait < time.Millisecond {
					wait = time.Millisecond
				}
				timer := time.NewTimer(wait)
				select {
				case <-s.ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				case <-s.uploadWake:
					timer.Stop()
				}
			}
		}()
	})
}

func (s *Service) ProcessUpload(ctx context.Context, id string) {
	j, client, ready := s.prepareUpload(ctx, id)
	if !ready {
		return
	}
	s.executeUpload(ctx, j, client)
}

func (s *Service) executeUpload(ctx context.Context, j models.UploadJob, client *uploads.Client) {
	defer func() {
		<-s.uploadSlots
		s.wakeUploads()
	}()
	runCtx, cancel := context.WithTimeout(ctx, 24*time.Hour)
	defer cancel()
	// The client passes on changed messages only, so each one is saved: a
	// throttle would drop a prompt that follows a stage change.
	out, err := client.Execute(runCtx, j, func(message string) error {
		j.Message = message
		return s.Store.SaveUpload(j)
	})
	// A result reported before a later error is kept for the review.
	j.TrackerResult = out.Tracker
	var limited *uploads.RateLimitError
	if errors.As(err, &limited) && j.RateLimitRetries >= maxRateLimitRetries {
		j.Status, j.Message = "needs_review", fmt.Sprintf("Upload-Assistant kept rate limiting requests after %d retries. Check it before reuploading.", j.RateLimitRetries)
	} else if errors.As(err, &limited) {
		delay := time.Minute
		for i := 0; i < j.RateLimitRetries && delay < time.Hour; i++ {
			delay *= 2
		}
		if delay > time.Hour {
			delay = time.Hour
		}
		if limited.RetryAfter > delay {
			delay = limited.RetryAfter
		}
		next := time.Now().UTC().Add(delay)
		j.RateLimitRetries++
		j.NextAttemptAt = &next
		j.Status, j.Message = "downloading", "Upload-Assistant is rate limiting requests; queued to retry after "+next.Format(time.RFC3339)
	} else if err != nil {
		j.Status, j.Message = "needs_review", err.Error()+". Check Upload-Assistant; this job will not be resubmitted automatically."
	} else if out.Tracker != nil {
		j.Status, j.Message = trackerOutcome(j.Tracker, out)
	} else if out.Code != 0 {
		j.Status, j.Message = "failed", fmt.Sprintf("Upload-Assistant exited with code %d. Check its WebUI before retrying.", out.Code)
	} else {
		j.Status, j.Message = "finished", "Upload-Assistant finished. Check its WebUI for the tracker outcome; a skipped upload can also exit successfully."
	}
	s.uploadMu.Lock()
	if err := s.Store.SaveUpload(j); err != nil {
		slog.Error("could not save upload outcome", "job", j.ID, "error", err)
	}
	s.uploadMu.Unlock()
}

// trackerOutcomes describe each tracker result UA reports. Anything but an
// upload fails the job, so skips wait for the user's review.
var trackerOutcomes = map[string]struct{ status, text string }{
	"success": {"finished", "Uploaded to %s"},
	"failed":  {"failed", "Upload to %s failed"},
	"dupe":    {"failed", "Not uploaded to %s (possible dupe)"},
	"skipped": {"failed", "Not uploaded to %s (skipped)"},
	"banned":  {"failed", "Not uploaded to %s (release group banned)"},
}

// trackerOutcome turns UA's result for the target into the job outcome. The
// tracker result outranks the exit code, which cannot tell a skip from an upload.
func trackerOutcome(tracker string, out uploads.Outcome) (string, string) {
	// The client passes on final results only, so every status has an entry.
	o := trackerOutcomes[out.Tracker.Status]
	message := fmt.Sprintf(o.text, tracker)
	if out.Tracker.Message != "" && out.Tracker.Status != "success" {
		message += ": " + out.Tracker.Message
	} else {
		message += "."
	}
	if out.Code != 0 {
		message += fmt.Sprintf(" Upload-Assistant exited with code %d.", out.Code)
	}
	return o.status, message
}

func (s *Service) prepareUpload(ctx context.Context, id string) (models.UploadJob, *uploads.Client, bool) {
	j, err := s.Store.Upload(id)
	if err != nil || j.Status != "downloading" {
		return j, nil, false
	}
	if j.NextAttemptAt != nil && time.Now().Before(*j.NextAttemptAt) {
		s.uploadMu.Lock()
		s.scheduleUploadCheck(*j.NextAttemptAt)
		s.uploadMu.Unlock()
		return j, nil, false
	}
	wait := func(message string) (models.UploadJob, *uploads.Client, bool) {
		j.Message = message
		if err := s.Store.SaveWaitingUpload(j); err != nil && !errors.Is(err, database.ErrUploadChanged) {
			slog.Error("could not save upload progress", "job", j.ID, "error", err)
		}
		return j, nil, false
	}
	v, err := s.Store.UploadSettings()
	if err != nil {
		return wait("Could not load upload settings")
	}
	if !v.Enabled {
		return wait("Uploads are disabled in Settings")
	}
	s.mu.RLock()
	settings := s.settings
	s.mu.RUnlock()
	if settings.QuiURL != j.QuiURL || v.URL != j.AssistantURL {
		return wait("Connection changed. Restore the original connection or cancel this job.")
	}
	c, err := qui.New(settings.QuiURL, settings.APIKey, s.Config.SearchTimeout)
	if err != nil {
		return wait("qui connection is unavailable")
	}
	pollCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	matched := []qui.Torrent{}
	if j.Tag == "" && j.Hash != "" {
		item, err := c.TorrentByHash(pollCtx, j.InstanceID, j.Hash)
		if err == nil {
			matched = append(matched, item)
		}
	} else {
		items, err := c.TaggedTorrents(pollCtx, j.InstanceID, j.Tag)
		if err != nil {
			return wait("Waiting for qui to report torrent status")
		}
		for _, item := range items {
			if slices.ContainsFunc(strings.Split(item.Tags, ","), func(tag string) bool { return strings.TrimSpace(tag) == j.Tag }) {
				matched = append(matched, item)
			}
		}
	}
	if len(matched) != 1 {
		return wait("Waiting for the source torrent in qui")
	}
	t := matched[0]
	if !torrentHashPattern.MatchString(t.Hash) || (j.Hash != "" && j.Hash != strings.ToLower(t.Hash)) {
		return wait("Torrent identity changed or is invalid; check qui")
	}
	j.Hash, j.Progress = strings.ToLower(t.Hash), t.Progress
	if t.Progress < 1 || t.AmountLeft != 0 {
		return wait("Downloading in qui")
	}
	if !slices.Contains([]string{"uploading", "stalledUP", "pausedUP", "stoppedUP", "queuedUP", "forcedUP"}, t.State) {
		return wait("Waiting for torrent checking or moving to finish")
	}
	complete, err := c.FilesComplete(pollCtx, j.InstanceID, j.Hash)
	if err != nil || !complete {
		return wait("Waiting for every torrent file to finish downloading")
	}
	j.ContentPath = t.ContentPath
	j.UploadPath, err = uploads.MapPath(t.ContentPath, j.InstanceID, j.Paths)
	if err != nil {
		return wait(err.Error())
	}
	client, err := uploads.New(v.URL, v.Token)
	if err != nil {
		return wait("Upload-Assistant credentials are unavailable")
	}
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	current, err := s.Store.Upload(id)
	if err != nil || current.Status != "downloading" || current.Attempt != j.Attempt || ctx.Err() != nil {
		return j, nil, false
	}
	latest, err := s.Store.UploadSettings()
	s.mu.RLock()
	connection := s.settings
	s.mu.RUnlock()
	if err != nil || !latest.Enabled || latest.URL != v.URL || latest.Token != v.Token || connection.QuiURL != settings.QuiURL || connection.APIKey != settings.APIKey {
		return wait("Settings changed while checking the download; checking again next poll")
	}
	after, err := s.Store.UploadDispatchAfter(v.URL, s.Config.UploadSubmissionInterval)
	if err != nil {
		return wait("Could not read Upload-Assistant queue timing")
	}
	if time.Now().Before(after) {
		s.scheduleUploadCheck(after)
		return wait("Download complete; queued for Upload-Assistant after " + after.Format(time.RFC3339))
	}
	select {
	case s.uploadSlots <- struct{}{}:
	default:
		return wait("Download complete; waiting for an Upload-Assistant worker")
	}
	now := time.Now().UTC()
	j.NextAttemptAt = nil
	j.SubmittedAt = &now
	j.Status, j.Message = "uploading", "Starting Upload-Assistant"
	claimed, err := s.Store.ClaimUpload(j)
	if err != nil {
		<-s.uploadSlots
		j.Status = "downloading"
		return wait("Could not reserve upload submission")
	}
	if !claimed {
		<-s.uploadSlots
		j.Status, j.Message = "needs_review", "This torrent and target already have an upload reservation. Check earlier queue jobs."
		_ = s.Store.SaveWaitingUpload(j)
		return j, nil, false
	}
	return j, client, true
}

// maxRateLimitRetries bounds automatic retries of a rate-limited submission,
// about three hours with the doubling delay.
const maxRateLimitRetries = 8

// scheduleUploadCheck is called with uploadMu held.
func (s *Service) scheduleUploadCheck(at time.Time) {
	if s.uploadDue.IsZero() || at.Before(s.uploadDue) {
		s.uploadDue = at
	}
}

// trustedForUpload reports IDs that Upload-Assistant may reuse without a
// manual TMDb ID: download history, which is bound to the torrent hash.
func trustedForUpload(v models.MonitorCandidate) bool {
	return v.Identity != nil && v.Identity.Found() && v.IdentityError == "" && v.Identity.Verified()
}

// confirmableForUpload reports IDs from a Sonarr/Radarr library name match,
// sent only when the upload request confirms them. A title lookup is left to
// Upload-Assistant's own search.
func confirmableForUpload(v models.MonitorCandidate) bool {
	return v.Identity != nil && v.Identity.Found() && v.IdentityError == "" && v.Identity.Source == models.IdentityArrParse
}

// uploadMedia picks the IDs Upload-Assistant receives: trusted (or confirmed)
// resolved IDs unless cleared, replaced entirely by a TMDb ID entered for the
// upload.
func uploadMedia(v models.MonitorCandidate, input models.MonitorUploadRequest) models.Media {
	media := v.Media
	trusted := trustedForUpload(v) || (input.ConfirmMediaIDs && confirmableForUpload(v))
	if !trusted || input.ClearMediaIDs || (input.TMDbID > 0 && input.TMDbID != media.TMDbID) {
		media.IMDbID, media.TMDbID, media.TVDbID = "", 0, 0
	}
	if input.TMDbID > 0 {
		media.TMDbID = input.TMDbID
	}
	return media
}

package search

import (
	"context"
	"log/slog"
	"time"
)

// startRetention prunes hourly: history and raw responses when retention is
// configured, and long-expired metadata always.
func (s *Service) startRetention() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
			err := s.Store.Prune(ctx, time.Now(), s.Config.HistoryRetention, s.Config.RawRetention)
			cancel()
			if err != nil && s.ctx.Err() == nil {
				slog.Error("history retention failed", "error", err)
			}
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

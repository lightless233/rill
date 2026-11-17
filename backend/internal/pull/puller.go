package pull

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/0x2E/fusion/internal/config"
	"github.com/0x2E/fusion/internal/model"
	"github.com/0x2E/fusion/internal/pullpolicy"
	"github.com/0x2E/fusion/internal/store"
	"golang.org/x/sync/semaphore"
)

// minScheduleDelay bounds the scheduler sleep so a stale next_check_at (e.g.
// after a failed fetch-state write) cannot spin the loop hot.
const minScheduleDelay = time.Second

type Puller struct {
	baseCtx      context.Context
	store        *store.Store
	config       *config.Config
	logger       *slog.Logger
	interval     time.Duration
	timeout      time.Duration
	maxBackoff   time.Duration
	concurrency  *semaphore.Weighted
	wake         chan struct{}
	feedsChanged func()
}

func (p *Puller) effectiveInterval(feed *model.Feed) time.Duration {
	if feed.RefreshIntervalSeconds != nil && *feed.RefreshIntervalSeconds > 0 {
		return time.Duration(*feed.RefreshIntervalSeconds) * time.Second
	}
	return p.interval
}

func New(st *store.Store, cfg *config.Config) *Puller {
	return NewWithContext(context.Background(), st, cfg)
}

// NewWithContext creates a Puller bound to baseCtx so process shutdown stops
// both periodic pulls and asynchronous manual/initial feed refreshes.
func NewWithContext(baseCtx context.Context, st *store.Store, cfg *config.Config) *Puller {
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	return &Puller{
		baseCtx:     baseCtx,
		store:       st,
		config:      cfg,
		logger:      slog.Default(),
		interval:    time.Duration(cfg.PullInterval) * time.Second,
		timeout:     time.Duration(cfg.PullTimeout) * time.Second,
		maxBackoff:  time.Duration(cfg.PullMaxBackoff) * time.Second,
		concurrency: semaphore.NewWeighted(int64(cfg.PullConcurrency)),
		wake:        make(chan struct{}, 1),
	}
}

// SetFeedsChanged installs the process-local notification callback used after
// a pull commits new items. It must be called before Start or any manual pull.
func (p *Puller) SetFeedsChanged(fn func()) {
	p.feedsChanged = fn
}

func (p *Puller) notifyFeedsChanged() {
	if p.feedsChanged != nil {
		p.feedsChanged()
	}
}

func (p *Puller) bindLifecycleContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if p.baseCtx == nil {
		return context.WithCancel(ctx)
	}

	mergedCtx, cancel := context.WithCancel(ctx)
	stopAfter := context.AfterFunc(p.baseCtx, cancel)
	if p.baseCtx.Err() != nil {
		cancel()
	}

	return mergedCtx, func() {
		stopAfter()
		cancel()
	}
}

func (p *Puller) withAttemptTimeout(parentCtx context.Context) (context.Context, context.CancelFunc) {
	if p.timeout > 0 {
		return context.WithTimeout(parentCtx, p.timeout)
	}
	return context.WithCancel(parentCtx)
}

// Wake prompts the scheduler to re-evaluate due feeds immediately. HTTP
// handlers call it after feed mutations (create/update/delete) so interval
// changes take effect without waiting out the previous sleep.
func (p *Puller) Wake() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// Start runs the pull scheduler until the context is cancelled. Each pass
// pulls the feeds that are due (per their next_check_at), then sleeps until
// the earliest next due time, or until Wake fires.
func (p *Puller) Start(ctx context.Context) error {
	lifecycleCtx, stopLifecycle := p.bindLifecycleContext(ctx)
	defer stopLifecycle()

	p.logger.Info("pull service started", "interval", p.interval, "timeout", p.timeout, "concurrency", p.config.PullConcurrency)

	for {
		dispatched := p.pullDue(lifecycleCtx)

		timer := time.NewTimer(p.scheduleDelay(dispatched > 0))
		select {
		case <-lifecycleCtx.Done():
			timer.Stop()
			p.logger.Info("pull service stopping")
			return lifecycleCtx.Err()
		case <-timer.C:
		case <-p.wake:
			timer.Stop()
		}
	}
}

// scheduleDelay returns how long the scheduler can sleep before any feed is
// due again, based on the fetch state persisted by the last pull. The sleep
// is capped at the global interval so a stale next_check_at (missed wake,
// host suspend, NTP step) delays re-evaluation by at most one interval.
func (p *Puller) scheduleDelay(dispatched bool) time.Duration {
	next, ok, err := p.store.NextWakeTime()
	if err != nil || !ok {
		if err != nil {
			p.logger.Error("failed to compute next wake time", "error", err)
		}
		// No non-suspended feeds; re-check after the global interval.
		return p.interval
	}
	if dispatched && next <= time.Now().Unix() {
		// Feeds were pulled this pass but the earliest due time is still in
		// the past: their fetch-state persist failed. Retry after the global
		// interval instead of hammering origins every second.
		return p.interval
	}
	delay := time.Until(time.Unix(next, 0))
	if delay < minScheduleDelay {
		return minScheduleDelay
	}
	if delay > p.interval {
		return p.interval
	}
	return delay
}

// pullDue fetches the feeds whose next_check_at has passed, concurrently with
// semaphore limiting. It returns how many feeds were dispatched.
func (p *Puller) pullDue(ctx context.Context) int {
	lifecycleCtx, stopLifecycle := p.bindLifecycleContext(ctx)
	defer stopLifecycle()
	if lifecycleCtx.Err() != nil {
		return 0
	}

	now := time.Now().Unix()
	feeds, err := p.store.ListDueFeeds(now)
	if err != nil {
		p.logger.Error("failed to list due feeds", "error", err)
		return 0
	}

	count, _ := p.dispatchFeeds(lifecycleCtx, feeds, func(feed *model.Feed) bool {
		state := pullpolicy.FeedRuntimeState{
			Suspended:           feed.Suspended,
			RetryAfterUntil:     feed.FetchState.RetryAfterUntil,
			NextCheckAt:         feed.FetchState.NextCheckAt,
			ConsecutiveFailures: feed.FetchState.ConsecutiveFailures,
			LastErrorAt:         feed.FetchState.LastErrorAt,
			LastCheckedAt:       feed.FetchState.LastCheckedAt,
		}
		return !pullpolicy.ShouldSkip(now, state, p.effectiveInterval(feed), p.maxBackoff)
	})
	return count
}

// pullFeed fetches single feed and saves new items.
func (p *Puller) pullFeed(parentCtx context.Context, feed *model.Feed) {
	if parentCtx.Err() != nil {
		return
	}

	p.logger.Debug("pulling feed", "feed_id", feed.ID, "feed_name", feed.Name)

	attemptCtx, cancelAttempt := p.withAttemptTimeout(parentCtx)
	defer cancelAttempt()

	result, err := FetchAndParse(attemptCtx, feed, p.timeout, p.config.AllowPrivateFeeds)
	checkedAt := time.Now().Unix()
	if err != nil {
		// Parent cancellation or parent expiry stops work cleanly without recording
		// a feed failure or triggering failure backoff; only attempt-level failures
		// while the parent context remains active count against consecutive_failures.
		if parentCtx.Err() != nil {
			p.logger.Debug("feed pull stopped by parent context", "feed_id", feed.ID, "feed_name", feed.Name, "error", parentCtx.Err())
			return
		}

		httpStatus := 0
		retryAfterUntil := int64(0)
		if result != nil {
			httpStatus = result.HTTPStatus
			retryAfterUntil = result.RetryAfterUntil
		}

		if err := p.store.UpdateFeedFetchFailure(feed.ID, store.UpdateFeedFetchFailureParams{
			CheckedAt:       checkedAt,
			HTTPStatus:      httpStatus,
			LastError:       err.Error(),
			RetryAfterUntil: retryAfterUntil,
			IntervalSeconds: int64(p.effectiveInterval(feed).Seconds()),
			MaxBackoff:      int64(p.maxBackoff.Seconds()),
		}); err != nil {
			p.logger.Error("failed to record failure", "feed_id", feed.ID, "error", err)
		}

		p.logger.Warn("failed to fetch feed", "feed_id", feed.ID, "feed_name", feed.Name, "status", httpStatus, "error", err)
		return
	}

	if result.NotModified {
		etag := result.ETag
		// Some servers reply 304 without echoing validators; keep the previous
		// ones so future conditional requests remain effective.
		if strings.TrimSpace(etag) == "" {
			etag = feed.FetchState.ETag
		}

		lastModified := result.LastModified
		if strings.TrimSpace(lastModified) == "" {
			lastModified = feed.FetchState.LastModified
		}

		cacheControl := result.CacheControl
		if strings.TrimSpace(cacheControl) == "" {
			cacheControl = feed.FetchState.CacheControl
		}

		expiresAt := result.ExpiresAt
		if expiresAt == 0 {
			expiresAt = feed.FetchState.ExpiresAt
		}

		nextCheckAt := pullpolicy.ComputeNextCheckAt(
			checkedAt,
			p.effectiveInterval(feed),
			p.maxBackoff,
			0,
			result.RetryAfterUntil,
			cacheControl,
			expiresAt,
		)

		if err := p.store.UpdateFeedFetchSuccess(feed.ID, store.UpdateFeedFetchSuccessParams{
			CheckedAt:       checkedAt,
			HTTPStatus:      result.HTTPStatus,
			ETag:            etag,
			LastModified:    lastModified,
			CacheControl:    cacheControl,
			ExpiresAt:       expiresAt,
			RetryAfterUntil: result.RetryAfterUntil,
			NextCheckAt:     nextCheckAt,
		}); err != nil {
			p.logger.Error("failed to persist not-modified state", "feed_id", feed.ID, "error", err)
			return
		}

		p.logger.Debug("feed not modified", "feed_id", feed.ID, "feed_name", feed.Name)
		return
	}

	nextCheckAt := pullpolicy.ComputeNextCheckAt(
		checkedAt,
		p.effectiveInterval(feed),
		p.maxBackoff,
		0,
		result.RetryAfterUntil,
		result.CacheControl,
		result.ExpiresAt,
	)

	inputs, err := p.prepareItemInputs(parentCtx, attemptCtx, feed, result.Items)
	if err != nil {
		if parentCtx.Err() != nil {
			p.logger.Debug("feed pull stopped by parent context", "feed_id", feed.ID, "feed_name", feed.Name, "error", parentCtx.Err())
			return
		}
		p.logger.Error("failed to prepare article content", "feed_id", feed.ID, "error", err)
		return
	}

	newCount, err := p.store.BatchCreateItemsIgnore(feed.ID, inputs)
	if err != nil {
		p.logger.Error("failed to batch create items", "feed_id", feed.ID, "error", err)
		return
	}
	if newCount > 0 {
		// BatchCreateItemsIgnore commits before returning, so the new unread
		// rows are visible to the feed-count query triggered by this signal.
		p.notifyFeedsChanged()
	}

	if err := p.store.UpdateFeedFetchSuccess(feed.ID, store.UpdateFeedFetchSuccessParams{
		CheckedAt:       checkedAt,
		HTTPStatus:      result.HTTPStatus,
		ETag:            result.ETag,
		LastModified:    result.LastModified,
		CacheControl:    result.CacheControl,
		ExpiresAt:       result.ExpiresAt,
		RetryAfterUntil: result.RetryAfterUntil,
		NextCheckAt:     nextCheckAt,
	}); err != nil {
		p.logger.Error("failed to update fetch state", "feed_id", feed.ID, "error", err)
		return
	}

	if strings.TrimSpace(feed.SiteURL) == "" && result.SiteURL != "" {
		if err := p.store.UpdateFeedSiteURLIfEmpty(feed.ID, result.SiteURL); err != nil {
			p.logger.Warn("failed to auto-fill site_url", "feed_id", feed.ID, "site_url", result.SiteURL, "error", err)
		}
	}

	if strings.TrimSpace(feed.Name) == "" || strings.TrimSpace(feed.Name) == strings.TrimSpace(feed.Link) {
		if err := p.store.UpdateFeedNameIfDefault(feed.ID, result.FeedTitle, feed.Link); err != nil {
			p.logger.Warn("failed to auto-fill feed name", "feed_id", feed.ID, "title", result.FeedTitle, "error", err)
		}
	}

	p.logger.Info("feed pulled successfully", "feed_id", feed.ID, "feed_name", feed.Name, "new_items", newCount)
}

// RefreshAll triggers refresh for all non-suspended feeds and waits until all
// started refresh jobs have completed. It bypasses backoff/interval skip logic.
// Concurrency is controlled by the same semaphore as periodic pulls.
func (p *Puller) RefreshAll(ctx context.Context) (int, error) {
	lifecycleCtx, stopLifecycle := p.bindLifecycleContext(ctx)
	defer stopLifecycle()

	feeds, err := p.store.ListFeeds()
	if err != nil {
		return 0, fmt.Errorf("list feeds: %w", err)
	}

	count, err := p.dispatchFeeds(lifecycleCtx, feeds, func(feed *model.Feed) bool {
		return !feed.Suspended
	})
	if err != nil {
		return count, err
	}

	p.Wake()
	return count, nil
}

func (p *Puller) dispatchFeeds(ctx context.Context, feeds []*model.Feed, shouldPull func(*model.Feed) bool) (int, error) {
	count := 0
	var wg sync.WaitGroup
	var acquireErr error

	for _, feed := range feeds {
		if !shouldPull(feed) {
			continue
		}

		if err := ctx.Err(); err != nil {
			acquireErr = err
			break
		}

		if err := p.concurrency.Acquire(ctx, 1); err != nil {
			acquireErr = err
			break
		}
		if err := ctx.Err(); err != nil {
			p.concurrency.Release(1)
			acquireErr = err
			break
		}

		count++
		wg.Add(1)
		go func(f *model.Feed) {
			defer wg.Done()
			defer p.concurrency.Release(1)
			p.pullFeed(ctx, f)
		}(feed)
	}

	wg.Wait()
	return count, acquireErr
}

// RefreshFeed manually triggers refresh for specific feed (bypasses skip logic).
// Used by HTTP handler for manual refresh requests.
func (p *Puller) RefreshFeed(ctx context.Context, feedID int64) error {
	lifecycleCtx, stopLifecycle := p.bindLifecycleContext(ctx)
	defer stopLifecycle()

	if err := lifecycleCtx.Err(); err != nil {
		return err
	}

	feed, err := p.store.GetFeed(feedID)
	if err != nil {
		return fmt.Errorf("get feed: %w", err)
	}

	if err := p.concurrency.Acquire(lifecycleCtx, 1); err != nil {
		return err
	}
	defer p.concurrency.Release(1)

	if err := lifecycleCtx.Err(); err != nil {
		return err
	}

	p.pullFeed(lifecycleCtx, feed)
	if lifecycleCtx.Err() != nil {
		return lifecycleCtx.Err()
	}

	// Manual refresh rewrites next_check_at (possibly earlier than the
	// sleeping timer's deadline); every writer outside pullDue must wake.
	p.Wake()
	return nil
}

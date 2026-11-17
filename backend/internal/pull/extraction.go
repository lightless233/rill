package pull

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/0x2E/fusion/internal/extract"
	"github.com/0x2E/fusion/internal/model"
	"github.com/0x2E/fusion/internal/store"
)

// prepareItemInputs builds the rows for one newly parsed feed response.
// Unconfigured feeds keep the old batch behaviour. Configured feeds check
// persisted identity before any article request, attempt extraction serially
// under the existing attempt deadline, and fall back to RSS content on failure
// or deadline exhaustion.
func (p *Puller) prepareItemInputs(
	parentCtx context.Context,
	attemptCtx context.Context,
	feed *model.Feed,
	items []*ParsedItem,
) ([]store.BatchCreateItemInput, error) {
	if strings.TrimSpace(feed.ContentSelector) == "" {
		inputs := make([]store.BatchCreateItemInput, 0, len(items))
		for _, item := range items {
			inputs = append(inputs, newItemInput(item, item.Content))
		}
		return inputs, nil
	}

	var (
		inputs            = make([]store.BatchCreateItemInput, 0, len(items))
		seen              = make(map[string]struct{}, len(items))
		newItems          int
		extracted         int
		fallbacks         int
		deadlineFallbacks int
		extractionStopped bool
	)

	for _, item := range items {
		if parentCtx.Err() != nil {
			return nil, parentCtx.Err()
		}
		if _, ok := seen[item.GUID]; ok {
			continue
		}
		seen[item.GUID] = struct{}{}

		exists, err := p.store.ItemExists(feed.ID, item.GUID)
		if err != nil {
			return nil, fmt.Errorf("check item identity: %w", err)
		}
		if exists {
			continue
		}

		newItems++
		content := item.Content
		switch {
		case extractionStopped || attemptCtx.Err() != nil:
			extractionStopped = true
			fallbacks++
			deadlineFallbacks++
		default:
			result, err := extract.Article(attemptCtx, extract.Options{
				URL:             item.Link,
				ContentSelector: feed.ContentSelector,
				ExcludeSelector: feed.ExcludeSelector,
				Proxy:           feed.Proxy,
				AllowPrivate:    p.config != nil && p.config.AllowPrivateFeeds,
				Timeout:         p.timeout,
			})
			if err == nil {
				content = result.Content
				extracted++
				break
			}
			if parentCtx.Err() != nil {
				return nil, parentCtx.Err()
			}
			if attemptCtx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
				extractionStopped = true
				fallbacks++
				deadlineFallbacks++
				p.logger.Warn(
					"article extraction stopped at attempt deadline",
					"feed_id", feed.ID,
					"feed_name", feed.Name,
					"guid", item.GUID,
					"error", err,
				)
				break
			}
			fallbacks++
			p.logger.Warn(
				"article extraction failed; using RSS content",
				"feed_id", feed.ID,
				"feed_name", feed.Name,
				"guid", item.GUID,
				"link", item.Link,
				"error", err,
			)
		}

		inputs = append(inputs, newItemInput(item, content))
	}

	if parentCtx.Err() != nil {
		return nil, parentCtx.Err()
	}
	if deadlineFallbacks > 0 {
		p.logger.Warn(
			"article extraction incomplete; saved RSS fallback",
			"feed_id", feed.ID,
			"feed_name", feed.Name,
			"reason", "attempt deadline",
			"fallback_count", deadlineFallbacks,
			"new_items", newItems,
			"extracted", extracted,
			"other_fallbacks", fallbacks-deadlineFallbacks,
		)
	}
	return inputs, nil
}

func newItemInput(item *ParsedItem, content string) store.BatchCreateItemInput {
	return store.BatchCreateItemInput{
		GUID:    item.GUID,
		Title:   item.Title,
		Link:    item.Link,
		Content: content,
		PubDate: item.PubDate,
	}
}

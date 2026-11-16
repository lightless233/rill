package store

import (
	"errors"
	"testing"
)

func TestArticleShareSnapshotAndScrub(t *testing.T) {
	st, _ := setupTestDB(t)
	defer closeStore(t, st)

	group := mustCreateGroup(t, st, "Share Group")
	feed := mustCreateFeed(t, st, group.ID, "Share Feed", "https://example.com/feed", "https://example.com", "")
	item := mustCreateItem(t, st, feed.ID, "share-guid", "Shared title", "https://example.com/article", "<p>Shared body</p>", 1700000000)

	share, err := st.CreateArticleShare(CreateArticleShareParams{
		Token:        "share-token",
		ItemID:       &item.ID,
		Title:        item.Title,
		Content:      item.Content,
		Link:         item.Link,
		FeedName:     feed.Name,
		PubDate:      item.PubDate,
		PasscodeHash: "passcode-hash",
		ExpiresAt:    1700003600,
	})
	if err != nil {
		t.Fatalf("CreateArticleShare() failed: %v", err)
	}

	if share.Token != "share-token" || share.Title != item.Title || share.Content != item.Content {
		t.Fatalf("unexpected share snapshot: %+v", share)
	}
	if share.ItemID == nil || *share.ItemID != item.ID {
		t.Fatalf("expected item association %d, got %+v", item.ID, share.ItemID)
	}
	if share.PasscodeHash != "passcode-hash" {
		t.Fatalf("expected stored passcode hash, got %q", share.PasscodeHash)
	}

	if err := st.DeleteFeed(feed.ID); err != nil {
		t.Fatalf("DeleteFeed() failed: %v", err)
	}

	afterDelete, err := st.GetArticleShareByToken("share-token")
	if err != nil {
		t.Fatalf("GetArticleShareByToken() after source deletion failed: %v", err)
	}
	if afterDelete.Title != item.Title || afterDelete.Content != item.Content || afterDelete.FeedName != feed.Name {
		t.Fatalf("snapshot changed after source deletion: %+v", afterDelete)
	}
	if afterDelete.ItemID != nil {
		t.Fatalf("expected source association to be cleared, got %d", *afterDelete.ItemID)
	}

	if err := st.ScrubExpiredArticleShares(1700003600); err != nil {
		t.Fatalf("ScrubExpiredArticleShares() failed: %v", err)
	}

	tombstone, err := st.GetArticleShareByToken("share-token")
	if err != nil {
		t.Fatalf("GetArticleShareByToken() after scrub failed: %v", err)
	}
	if tombstone.Title != "" || tombstone.Content != "" || tombstone.Link != "" || tombstone.FeedName != "" {
		t.Fatalf("expected article fields to be scrubbed, got %+v", tombstone)
	}
	if tombstone.PasscodeHash != "" {
		t.Fatalf("expected passcode hash to be scrubbed, got %q", tombstone.PasscodeHash)
	}
	if tombstone.Token != "share-token" || tombstone.ExpiresAt != 1700003600 {
		t.Fatalf("expected token/expiry tombstone, got %+v", tombstone)
	}
}

func TestGetArticleShareByTokenNotFound(t *testing.T) {
	st, _ := setupTestDB(t)
	defer closeStore(t, st)

	_, err := st.GetArticleShareByToken("missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCreateArticleShareRejectsDuplicateToken(t *testing.T) {
	st, _ := setupTestDB(t)
	defer closeStore(t, st)

	params := CreateArticleShareParams{
		Token:     "duplicate-token",
		Title:     "Title",
		Content:   "Content",
		ExpiresAt: 1700003600,
	}
	if _, err := st.CreateArticleShare(params); err != nil {
		t.Fatalf("first CreateArticleShare() failed: %v", err)
	}
	if _, err := st.CreateArticleShare(params); err == nil {
		t.Fatal("expected duplicate token to be rejected")
	}
}

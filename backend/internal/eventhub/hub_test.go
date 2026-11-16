package eventhub

import "testing"

func TestHubNotifiesEverySubscriber(t *testing.T) {
	hub := New()
	defer hub.Close()

	first, unsubscribeFirst := hub.Subscribe()
	defer unsubscribeFirst()
	second, unsubscribeSecond := hub.Subscribe()
	defer unsubscribeSecond()

	hub.Notify()

	for name, notifications := range map[string]<-chan struct{}{
		"first":  first,
		"second": second,
	} {
		select {
		case _, ok := <-notifications:
			if !ok {
				t.Fatalf("%s subscriber channel was closed", name)
			}
		default:
			t.Fatalf("%s subscriber did not receive notification", name)
		}
	}
}

func TestHubCoalescesPendingNotifications(t *testing.T) {
	hub := New()
	defer hub.Close()

	notifications, unsubscribe := hub.Subscribe()
	defer unsubscribe()

	hub.Notify()
	hub.Notify()
	hub.Notify()

	if _, ok := <-notifications; !ok {
		t.Fatal("subscriber channel was closed")
	}
	select {
	case <-notifications:
		t.Fatal("expected coalesced notifications")
	default:
	}
}

func TestHubUnsubscribeStopsNotifications(t *testing.T) {
	hub := New()
	notifications, unsubscribe := hub.Subscribe()
	unsubscribe()
	unsubscribe()

	hub.Notify()

	if _, ok := <-notifications; ok {
		t.Fatal("expected unsubscribed channel to be closed")
	}
}

func TestHubCloseClosesSubscribersAndRejectsNewOnes(t *testing.T) {
	hub := New()
	notifications, unsubscribe := hub.Subscribe()
	defer unsubscribe()

	hub.Close()
	hub.Close()

	if _, ok := <-notifications; ok {
		t.Fatal("expected close to close subscriber channel")
	}

	late, lateUnsubscribe := hub.Subscribe()
	defer lateUnsubscribe()
	if _, ok := <-late; ok {
		t.Fatal("expected subscribe after close to return a closed channel")
	}
}

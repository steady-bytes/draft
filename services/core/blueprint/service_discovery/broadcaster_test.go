package service_discovery

import (
	"strconv"
	"testing"
	"time"

	sdv1 "github.com/steady-bytes/draft/api/core/registry/service_discovery/v1"
)

// awaitDrain waits for a wake signal (or the timeout) and returns what Drain(id) has, failing the
// test if nothing arrives in time.
func awaitDrain(t *testing.T, b *Broadcaster, id string, wake <-chan struct{}) []*ProcessEvent {
	t.Helper()
	select {
	case <-wake:
		return b.Drain(id)
	case <-time.After(time.Second):
		t.Fatal("no wake signal received")
		return nil
	}
}

func assertNoWake(t *testing.T, wake <-chan struct{}) {
	t.Helper()
	select {
	case <-wake:
		t.Fatal("unexpected wake signal: nothing should be pending")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestPublishDeliversToASubscriber(t *testing.T) {
	b := NewBroadcaster()
	id, wake := b.Subscribe()

	b.Publish(&sdv1.Process{Pid: "p1", Name: "crud"})

	events := awaitDrain(t, b, id, wake)
	if len(events) != 1 || events[0].Process.GetPid() != "p1" {
		t.Fatalf("events = %+v, want one event for p1", events)
	}
}

// TestBurstOfHeartbeatsForTheSamePidCoalescesToTheLatest is the regression test for the bug this
// fix addresses: confirmed live (querying Blueprint directly bypassed a stalled web client and
// showed fresh, correct server-side data even while the UI showed stale processes) that a burst of
// publishes for the same pid, arriving faster than a subscriber drains, used to silently drop all
// but whichever ones happened to land while the old fixed-size buffered channel had room --
// including possibly the final, most-current one. Coalescing means the *latest* state always wins
// regardless of how many updates arrived before the subscriber got around to draining.
func TestBurstOfHeartbeatsForTheSamePidCoalescesToTheLatest(t *testing.T) {
	b := NewBroadcaster()
	id, wake := b.Subscribe()

	// Simulate 50 rapid heartbeats for the same process arriving before the subscriber drains even
	// once -- far more than the old implementation's 16-slot buffer, which would have dropped the
	// overflow non-blockingly and silently.
	for i := 0; i < 50; i++ {
		b.Publish(&sdv1.Process{Pid: "p1", Name: "crud", IpAddress: intToAddr(i)})
	}

	events := awaitDrain(t, b, id, wake)
	if len(events) != 1 {
		t.Fatalf("events = %d, want exactly 1 (coalesced): %+v", len(events), events)
	}
	if got := events[0].Process.GetIpAddress(); got != intToAddr(49) {
		t.Errorf("coalesced event = %q, want the last one published (%q) — a burst must never lose the most current state", got, intToAddr(49))
	}
}

// TestDistinctPidsAreNeverCoalescedTogether guards against an implementation that accidentally
// coalesces across processes instead of only within one pid's own updates.
func TestDistinctPidsAreNeverCoalescedTogether(t *testing.T) {
	b := NewBroadcaster()
	id, wake := b.Subscribe()

	b.Publish(&sdv1.Process{Pid: "p1", Name: "crud"})
	b.Publish(&sdv1.Process{Pid: "p2", Name: "beacon"})
	b.Publish(&sdv1.Process{Pid: "p1", Name: "crud", HealthState: sdv1.ProcessHealthState_PROCESS_UNHEALTHY})

	events := awaitDrain(t, b, id, wake)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (one per distinct pid): %+v", len(events), events)
	}
	byPid := map[string]*ProcessEvent{}
	for _, e := range events {
		byPid[e.Process.GetPid()] = e
	}
	if byPid["p1"] == nil || byPid["p1"].Process.GetHealthState() != sdv1.ProcessHealthState_PROCESS_UNHEALTHY {
		t.Errorf("p1's event should be its latest (unhealthy) state, got %+v", byPid["p1"])
	}
	if byPid["p2"] == nil {
		t.Error("p2's event was lost")
	}
}

func TestPublishRemovedIsDeliveredAndCoalescesWithAnEarlierUpdate(t *testing.T) {
	b := NewBroadcaster()
	id, wake := b.Subscribe()

	b.Publish(&sdv1.Process{Pid: "p1", Name: "crud"})
	b.PublishRemoved("p1")

	events := awaitDrain(t, b, id, wake)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1 (the removal supersedes the earlier update)", len(events))
	}
	if !events[0].Removed {
		t.Error("expected the coalesced event to be the removal, not the earlier update")
	}
}

func TestDrainReturnsNothingWhenNothingIsPending(t *testing.T) {
	b := NewBroadcaster()
	id, _ := b.Subscribe()

	if events := b.Drain(id); events != nil {
		t.Errorf("Drain with nothing published = %+v, want nil", events)
	}
}

func TestEachSubscriberGetsItsOwnCopy(t *testing.T) {
	b := NewBroadcaster()
	id1, wake1 := b.Subscribe()
	id2, wake2 := b.Subscribe()

	b.Publish(&sdv1.Process{Pid: "p1", Name: "crud"})

	e1 := awaitDrain(t, b, id1, wake1)
	e2 := awaitDrain(t, b, id2, wake2)
	if len(e1) != 1 || len(e2) != 1 {
		t.Fatalf("both subscribers should independently receive the event: got %d and %d", len(e1), len(e2))
	}
}

func TestUnsubscribeStopsFurtherDeliveryAndIsIdempotent(t *testing.T) {
	b := NewBroadcaster()
	id, wake := b.Subscribe()

	b.Unsubscribe(id)
	b.Unsubscribe(id) // must not panic

	b.Publish(&sdv1.Process{Pid: "p1", Name: "crud"})
	assertNoWake(t, wake)

	if events := b.Drain(id); events != nil {
		t.Errorf("Drain after Unsubscribe = %+v, want nil (unknown subscriber)", events)
	}
}

// TestConcurrentPublishAndDrainDoesNotRace exercises the mutex discipline under `go test -race`:
// many goroutines publishing for overlapping pids while the subscriber repeatedly drains.
func TestConcurrentPublishAndDrainDoesNotRace(t *testing.T) {
	b := NewBroadcaster()
	id, wake := b.Subscribe()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 500; i++ {
			b.Publish(&sdv1.Process{Pid: intToAddr(i % 10), Name: "crud"})
		}
	}()

	total := 0
	timeout := time.After(2 * time.Second)
loop:
	for {
		select {
		case <-wake:
			total += len(b.Drain(id))
		case <-done:
			// Drain whatever's left after the publisher finishes.
			for {
				select {
				case <-wake:
					total += len(b.Drain(id))
				default:
					break loop
				}
			}
		case <-timeout:
			t.Fatal("timed out waiting for publishes to finish")
		}
	}
	if total == 0 {
		t.Fatal("drained zero events from 500 publishes")
	}
	// total <= 500 always (coalescing only ever reduces count) and > 0 confirms delivery happened;
	// the exact count is timing-dependent (how much coalescing occurred), which is the point.
	if total > 500 {
		t.Errorf("total drained = %d, want <= 500 (coalescing should only ever reduce the count)", total)
	}
}

func intToAddr(i int) string {
	return "10.0.0." + strconv.Itoa(i)
}

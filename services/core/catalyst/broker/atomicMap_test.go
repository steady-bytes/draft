package broker

import (
	"testing"
	"time"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
)

// recv reads one message off ch, failing the test if none arrives promptly — every case below
// delivers synchronously (Broadcast's send is non-blocking into an already-buffered channel), so a
// short timeout is just a guard against the test hanging on a real regression, not a legitimate
// wait.
func recv(t *testing.T, ch chan *acv1.CloudEvent) *acv1.CloudEvent {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(time.Second):
		t.Fatal("no message received")
		return nil
	}
}

func assertEmpty(t *testing.T, ch chan *acv1.CloudEvent, who string) {
	t.Helper()
	select {
	case msg := <-ch:
		t.Fatalf("%s: unexpectedly received %+v", who, msg)
	default:
	}
}

func TestBroadcastReachesEveryConsumerOfTheSameType(t *testing.T) {
	// The bug this guards against: routing used to key on the CloudEvent envelope's own Go type
	// name (always the same value for every message), and delivery went through one shared,
	// unbuffered channel every open Consume stream read from — so with two consumers of the SAME
	// declared type open at once, a produced event reached only one of them, chosen essentially at
	// random, not both. See atomicMap.Broadcast's own doc comment for the full history.
	am := newAtomicMap()
	_, a := am.Subscribe("widget.created")
	_, b := am.Subscribe("widget.created")

	msg := &acv1.CloudEvent{Type: "widget.created", Id: "1"}
	am.Broadcast(msg)

	if got := recv(t, a); got != msg {
		t.Errorf("consumer a: got %+v, want the same message", got)
	}
	if got := recv(t, b); got != msg {
		t.Errorf("consumer b: got %+v, want the same message", got)
	}
}

func TestBroadcastSkipsConsumersOfADifferentType(t *testing.T) {
	am := newAtomicMap()
	_, widgets := am.Subscribe("widget.created")
	_, gadgets := am.Subscribe("gadget.created")

	am.Broadcast(&acv1.CloudEvent{Type: "widget.created"})

	recv(t, widgets) // must arrive
	assertEmpty(t, gadgets, "a gadget.created consumer")
}

func TestWildcardConsumerReceivesEveryType(t *testing.T) {
	// An empty declared type is a wildcard — e.g. `dctl broker consume` run with no --type flag,
	// which is meant to watch every event in the cluster.
	am := newAtomicMap()
	_, wildcard := am.Subscribe("")
	_, typed := am.Subscribe("widget.created")

	am.Broadcast(&acv1.CloudEvent{Type: "widget.created"})
	recv(t, wildcard)
	recv(t, typed)

	am.Broadcast(&acv1.CloudEvent{Type: "gadget.created"})
	recv(t, wildcard)
	assertEmpty(t, typed, "a widget.created consumer given a gadget.created event")
}

func TestUnsubscribeStopsFurtherDelivery(t *testing.T) {
	am := newAtomicMap()
	id, ch := am.Subscribe("widget.created")

	am.Broadcast(&acv1.CloudEvent{Type: "widget.created"})
	recv(t, ch)

	am.Unsubscribe(id)
	am.Broadcast(&acv1.CloudEvent{Type: "widget.created"})
	assertEmpty(t, ch, "an unsubscribed consumer")

	// Idempotent: unsubscribing an id twice (or one that was never subscribed) must not panic.
	am.Unsubscribe(id)
}

func TestASlowConsumerDoesNotBlockDeliveryToOthers(t *testing.T) {
	// Broadcast must not block on a consumer whose channel is already full (a stalled network
	// write, a client that stopped reading) — it should skip that one and keep delivering to
	// everyone else, the same non-blocking-drop discipline observerRegistry.broadcast already uses.
	am := newAtomicMap()
	_, slow := am.Subscribe("widget.created")
	_, fine := am.Subscribe("widget.created")

	// Fill the slow consumer's buffer completely without draining it.
	for i := 0; i < subChanCapacity; i++ {
		am.Broadcast(&acv1.CloudEvent{Type: "widget.created", Id: "filler"})
	}
	for i := 0; i < subChanCapacity; i++ {
		recv(t, fine) // drain the other consumer's copies of the same filler broadcasts
	}

	done := make(chan struct{})
	go func() {
		am.Broadcast(&acv1.CloudEvent{Type: "widget.created", Id: "last"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Broadcast blocked on a full consumer channel")
	}

	if got := recv(t, fine); got.GetId() != "last" {
		t.Errorf("the non-full consumer should still receive it: got id %q", got.GetId())
	}
	// slow's buffer is still full of the original filler messages (it was never drained) — the
	// point of this test is that the "last" broadcast was dropped for it, not delivered once room
	// freed up, so draining what's actually buffered must never turn up "last".
	for i := 0; i < subChanCapacity; i++ {
		if got := recv(t, slow); got.GetId() == "last" {
			t.Fatalf("the already-full consumer should have been skipped for \"last\", not queued behind the filler")
		}
	}
	assertEmpty(t, slow, "the already-full consumer, after draining exactly its capacity of filler messages")
}

func TestRegistrationsAreIndependentOfDeliverySubscriptions(t *testing.T) {
	// AddRegistration/Registrations/ConsumerCount are the topology bookkeeping GetTopology reads —
	// keyed by each consumer's own declared (type, source), entirely separate from the delivery
	// subscriptions above (which are keyed by an internal counter, not by source name at all).
	am := newAtomicMap()
	am.AddRegistration("widget.created", "svc-a")
	am.AddRegistration("widget.created", "svc-b")
	am.AddRegistration("widget.created", "svc-a") // duplicate: ignored

	if got := am.Registrations()["widget.created"]; len(got) != 2 {
		t.Fatalf("registrations = %v, want 2 distinct sources", got)
	}
	if got := am.ConsumerCount(); got != 2 {
		t.Fatalf("ConsumerCount = %d, want 2", got)
	}

	am.RemoveRegistration("widget.created", "svc-a")
	if got := am.Registrations()["widget.created"]; len(got) != 1 || got[0] != "svc-b" {
		t.Fatalf("after removing svc-a: %v", got)
	}

	am.RemoveRegistration("widget.created", "svc-b")
	if _, ok := am.Registrations()["widget.created"]; ok {
		t.Fatalf("an event type with no consumers left should not appear in Registrations at all")
	}
}

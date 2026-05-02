package smtpdebug

import "testing"

func TestStoreKeepsNewestEvents(t *testing.T) {
	store := New(2)
	store.Add(Event{Type: EventConnect, RemoteAddr: "one"})
	store.Add(Event{Type: EventHelo, RemoteAddr: "two"})
	store.Add(Event{Type: EventMailStored, RemoteAddr: "three"})

	events := store.List()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].RemoteAddr != "three" || events[1].RemoteAddr != "two" {
		t.Fatalf("expected newest events first, got %#v", events)
	}
	if events[0].ID == "" || events[0].Time.IsZero() {
		t.Fatalf("expected generated id and time: %#v", events[0])
	}
}

func TestStoreClear(t *testing.T) {
	store := New(0)
	store.Add(Event{Type: EventConnect})
	store.Clear()
	if len(store.List()) != 0 {
		t.Fatal("expected clear to remove all events")
	}
}

func TestSMTPPortAndSpecialIPFlags(t *testing.T) {
	if port := smtpPort("0.0.0.0:25"); port != "25" {
		t.Fatalf("unexpected port: %q", port)
	}
	if port := smtpPort(":2525"); port != "2525" {
		t.Fatalf("unexpected short port: %q", port)
	}
	flags := specialIPFlags("198.18.5.76")
	if len(flags) == 0 {
		t.Fatal("expected 198.18.5.76 to be reported as special-use")
	}
}

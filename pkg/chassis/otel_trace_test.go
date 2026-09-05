package chassis

import "testing"

// TestSpanLogBuffer_Cap confirms the memory-growth cap the WideEvent spec
// itself flagged as a required, not optional, part of Phase 5: a
// pathological span that logs excessively must not grow spanLogBuffer
// unboundedly — it should retain exactly maxBufferedSpanLogs lines and
// count the rest as overflow, not silently drop the count or panic.
func TestSpanLogBuffer_Cap(t *testing.T) {
	b := newSpanLogBuffer()

	const total = maxBufferedSpanLogs + 50
	for i := 0; i < total; i++ {
		b.append(spanLogLine{severity: "info", body: "line"})
	}

	lines, overflow := b.drain()
	if len(lines) != maxBufferedSpanLogs {
		t.Fatalf("got %d buffered lines, want %d", len(lines), maxBufferedSpanLogs)
	}
	if overflow != 50 {
		t.Fatalf("got overflow %d, want 50", overflow)
	}
}

// TestSpanLogBuffer_UnderCap confirms the common case isn't accidentally
// padded or truncated: fewer lines than the cap should drain exactly as
// appended, with zero overflow.
func TestSpanLogBuffer_UnderCap(t *testing.T) {
	b := newSpanLogBuffer()

	b.append(spanLogLine{severity: "info", body: "one"})
	b.append(spanLogLine{severity: "error", body: "two"})

	lines, overflow := b.drain()
	if len(lines) != 2 {
		t.Fatalf("got %d buffered lines, want 2", len(lines))
	}
	if overflow != 0 {
		t.Fatalf("got overflow %d, want 0", overflow)
	}
	if lines[0].body != "one" || lines[1].body != "two" {
		t.Fatalf("lines out of order or wrong content: %+v", lines)
	}
}

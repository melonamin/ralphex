package web

import (
	"fmt"

	"github.com/umputun/ralphex/pkg/processor"
	"github.com/umputun/ralphex/pkg/progress"
)

// BroadcastLogger wraps a processor.Logger and broadcasts events to SSE clients.
// implements the decorator pattern - all calls are forwarded to the inner logger
// while also being converted to events for web streaming.
type BroadcastLogger struct {
	inner  processor.Logger
	hub    *Hub
	buffer *Buffer
	phase  progress.Phase
}

// NewBroadcastLogger creates a logger that wraps inner and broadcasts to hub/buffer.
func NewBroadcastLogger(inner processor.Logger, hub *Hub, buffer *Buffer) *BroadcastLogger {
	return &BroadcastLogger{
		inner:  inner,
		hub:    hub,
		buffer: buffer,
		phase:  progress.PhaseTask,
	}
}

// SetPhase sets the current execution phase for color coding.
func (b *BroadcastLogger) SetPhase(phase progress.Phase) {
	b.phase = phase
	b.inner.SetPhase(phase)
}

// Print writes a timestamped message and broadcasts it.
func (b *BroadcastLogger) Print(format string, args ...any) {
	b.inner.Print(format, args...)
	b.broadcast(NewOutputEvent(b.phase, formatText(format, args...)))
}

// PrintRaw writes without timestamp and broadcasts it.
func (b *BroadcastLogger) PrintRaw(format string, args ...any) {
	b.inner.PrintRaw(format, args...)
	b.broadcast(NewOutputEvent(b.phase, formatText(format, args...)))
}

// PrintSection writes a section header and broadcasts it.
func (b *BroadcastLogger) PrintSection(name string) {
	b.inner.PrintSection(name)
	b.broadcast(NewSectionEvent(b.phase, name))
}

// PrintAligned writes text with timestamp on each line and broadcasts it.
func (b *BroadcastLogger) PrintAligned(text string) {
	b.inner.PrintAligned(text)
	b.broadcast(NewOutputEvent(b.phase, text))
}

// Path returns the progress file path.
func (b *BroadcastLogger) Path() string {
	return b.inner.Path()
}

// broadcast sends an event to both the buffer (for late-joining clients) and the hub (for live clients).
func (b *BroadcastLogger) broadcast(e Event) {
	b.buffer.Add(e)
	b.hub.Broadcast(e)
}

// formatText formats a string with args, like fmt.Sprintf.
func formatText(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

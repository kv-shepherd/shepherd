package domain

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

// EventHandler processes a domain event.
type EventHandler func(ctx context.Context, event *DomainEvent) error

// EventDispatcher routes domain events to registered handlers.
// ADR-0009: Domain Event Pattern (Claim-check, not Event Sourcing).
type EventDispatcher struct {
	handlers map[EventType][]EventHandler
	mu       sync.RWMutex
}

// NewEventDispatcher creates a new EventDispatcher.
func NewEventDispatcher() *EventDispatcher {
	return &EventDispatcher{
		handlers: make(map[EventType][]EventHandler),
	}
}

// Register registers a handler for a specific event type.
func (d *EventDispatcher) Register(eventType EventType, handler EventHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[eventType] = append(d.handlers[eventType], handler)
}

// Dispatch dispatches an event to all registered handlers.
// All handlers are called sequentially. If any handler fails, the error is logged
// but remaining handlers are still executed (best-effort delivery).
func (d *EventDispatcher) Dispatch(ctx context.Context, event *DomainEvent) error {
	d.mu.RLock()
	handlers := d.handlers[event.EventType]
	d.mu.RUnlock()

	if len(handlers) == 0 {
		logger.Warn("No handlers registered for event type",
			zap.String("event_type", string(event.EventType)),
			zap.String("event_id", event.EventID),
		)
		return nil
	}

	var firstErr error
	for _, handler := range handlers {
		if err := handler(ctx, event); err != nil {
			logger.Error("Event handler failed",
				zap.String("event_type", string(event.EventType)),
				zap.String("event_id", event.EventID),
				zap.Error(err),
			)
			if firstErr == nil {
				firstErr = fmt.Errorf("handler for %s failed: %w", event.EventType, err)
			}
		}
	}

	return firstErr
}

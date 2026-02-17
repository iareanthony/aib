package parser

import (
	"context"
	"time"
)

const defaultExternalCommandTimeout = 30 * time.Second

// WithDefaultCommandTimeout returns a context with a default timeout only when
// the parent context has no deadline set.
func WithDefaultCommandTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := parent.Deadline(); hasDeadline {
		return parent, func() {}
	}

	return context.WithTimeout(parent, defaultExternalCommandTimeout)
}

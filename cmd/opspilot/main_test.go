package main

import (
	"context"
	"testing"
)

func TestNewRunContextCanBeCanceled(t *testing.T) {
	ctx, stop := newRunContext(context.Background())
	stop()

	select {
	case <-ctx.Done():
	default:
		t.Fatal("run context was not canceled")
	}
}

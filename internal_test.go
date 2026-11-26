// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess

import (
	"errors"
	"fmt"
	"testing"

	"code.hybscloud.com/iox"
	"code.hybscloud.com/kont"
)

// delayedSend returns firstErr once. A retry runs the real send.
type delayedSend struct {
	Send[int]
	firstErr error
	calls    int
}

func (s *delayedSend) DispatchSession(ctx *sessionContext) (kont.Resumed, error) {
	s.calls++
	if s.calls == 1 {
		return nil, s.firstErr
	}
	return s.Send.DispatchSession(ctx)
}

func TestPairedRunnersRetryAfterBothRolesWait(t *testing.T) {
	blocked := fmt.Errorf("delayed send: %w", iox.ErrWouldBlock)
	t.Run("RunExpr", func(t *testing.T) {
		send := &delayedSend{Send: Send[int]{Value: 42}, firstErr: blocked}
		_, got := RunExpr(kont.ExprPerform(send), kont.ExprPerform(Recv[int]{}))
		if got != 42 || send.calls != 2 {
			t.Fatalf("received %d after %d send attempts, want 42 after 2", got, send.calls)
		}
	})
	t.Run("RunErrorExpr", func(t *testing.T) {
		send := &delayedSend{Send: Send[int]{Value: 42}, firstErr: blocked}
		sent, received, thrown := RunErrorExpr[string](kont.ExprPerform(send), kont.ExprPerform(Recv[int]{}))
		if got, ok := received.GetRight(); thrown != nil || !sent.IsRight() || !ok || got != 42 {
			t.Fatalf("sent=%v received=%v thrown=%v", sent, received, thrown)
		}
		if send.calls != 2 {
			t.Fatalf("send attempts = %d, want 2", send.calls)
		}
	})
}

func TestRunnersRejectUnexpectedDispatchError(t *testing.T) {
	for _, tt := range []struct {
		name string
		run  func(*delayedSend)
	}{
		{"ExecExpr", func(op *delayedSend) {
			ep, _ := New()
			ExecExpr(ep, kont.ExprPerform(op))
		}},
		{"ExecErrorExpr", func(op *delayedSend) {
			ep, _ := New()
			ExecErrorExpr[string](ep, kont.ExprPerform(op))
		}},
		{"RunExpr/left", func(op *delayedSend) {
			RunExpr(kont.ExprPerform(op), ExprCloseDone(struct{}{}))
		}},
		{"RunExpr/right", func(op *delayedSend) {
			RunExpr(ExprCloseDone(struct{}{}), kont.ExprPerform(op))
		}},
		{"RunErrorExpr/left", func(op *delayedSend) {
			RunErrorExpr[string](kont.ExprPerform(op), ExprCloseDone(struct{}{}))
		}},
		{"RunErrorExpr/right", func(op *delayedSend) {
			RunErrorExpr[string](ExprCloseDone(struct{}{}), kont.ExprPerform(op))
		}},
	} {
		for _, err := range []error{iox.ErrMore, errors.New("boom")} {
			t.Run(tt.name+"/"+err.Error(), func(t *testing.T) {
				op := &delayedSend{firstErr: err}
				defer func() {
					if got, want := recover(), "sess: dispatch returned unexpected error: "+err.Error(); got != want {
						t.Errorf("panic = %v, want %q", got, want)
					}
					if op.calls != 1 {
						t.Errorf("dispatch calls = %d, want 1", op.calls)
					}
				}()
				tt.run(op)
			})
		}
	}
}

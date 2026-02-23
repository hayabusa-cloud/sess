// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess

import (
	"code.hybscloud.com/iox"
	"code.hybscloud.com/kont"
)

// sessionErrorHandler handles both session and error effects.
// Session ops wait on ErrWouldBlock via iox.Backoff. Error ops short-circuit on Throw.
// Value type: passed to evalFrames on the stack, avoiding heap allocation.
type sessionErrorHandler[E, A any] struct {
	ctx    *sessionContext
	errCtx *kont.ErrorContext[E]
}

// Dispatch implements kont.Handler for the composed Session+Error handler.
// Dispatch order: Session → Error.
func (h sessionErrorHandler[E, A]) Dispatch(op kont.Operation) (kont.Resumed, bool) {
	if sop, ok := op.(sessionDispatcher); ok {
		return dispatchWait(h.ctx, sop), true
	}
	if eop, ok := op.(interface {
		DispatchError(ctx *kont.ErrorContext[E]) (kont.Resumed, bool)
	}); ok {
		v, _ := eop.DispatchError(h.errCtx)
		if h.errCtx.HasErr {
			return kont.Left[E, A](h.errCtx.Err), false
		}
		return v, true
	}
	panic("sess: unhandled effect in SessionErrorHandler")
}

// ExecError runs a session protocol with error handling on a pre-created endpoint.
// Returns Either[E, R] — Right on success, Left on Throw.
// Blocks on iox.ErrWouldBlock via adaptive backoff, without spawning goroutines
// or creating channels.
func ExecError[E, R any](ep *Endpoint, protocol kont.Eff[R]) kont.Either[E, R] {
	wrapped := kont.Map[kont.Resumed, R, kont.Either[E, R]](protocol, func(r R) kont.Either[E, R] {
		return kont.Right[E, R](r)
	})
	var errCtx kont.ErrorContext[E]
	h := sessionErrorHandler[E, R]{ctx: &ep.ctx, errCtx: &errCtx}
	return kont.Handle(wrapped, h)
}

// ExecErrorExpr runs an Expr session protocol with error handling on a pre-created endpoint.
// Returns Either[E, R] — Right on success, Left on Throw.
// Blocks on iox.ErrWouldBlock via adaptive backoff, without spawning goroutines
// or creating channels.
func ExecErrorExpr[E, R any](ep *Endpoint, protocol kont.Expr[R]) kont.Either[E, R] {
	wrapped := kont.ExprMap(protocol, func(r R) kont.Either[E, R] {
		return kont.Right[E, R](r)
	})
	var errCtx kont.ErrorContext[E]
	h := sessionErrorHandler[E, R]{ctx: &ep.ctx, errCtx: &errCtx}
	return kont.HandleExpr(wrapped, h)
}

// RunError creates a session pair, runs both Cont-world protocols with error
// handling, and returns both results as Either values. Interleaves execution
// on the calling goroutine using adaptive backoff (iox.Backoff).
// Does not spawn goroutines or create channels.
func RunError[E, A, B any](a kont.Eff[A], b kont.Eff[B]) (kont.Either[E, A], kont.Either[E, B]) {
	return RunErrorExpr[E](Reify(a), Reify(b))
}

// RunErrorExpr creates a session pair, runs both Expr-world protocols with
// error handling, and returns both results as Either values. Interleaves
// execution on the calling goroutine using adaptive backoff (iox.Backoff).
// Does not spawn goroutines or create channels.
func RunErrorExpr[E, A, B any](a kont.Expr[A], b kont.Expr[B]) (kont.Either[E, A], kont.Either[E, B]) {
	epA, epB := New()
	resultA, suspA := StepError[E, A](a)
	resultB, suspB := StepError[E, B](b)
	var bo iox.Backoff
	for suspA != nil || suspB != nil {
		progress := false
		if suspA != nil {
			var err error
			resultA, suspA, err = AdvanceError[E](epA, suspA)
			if err == nil {
				progress = true
			}
		}
		if suspB != nil {
			var err error
			resultB, suspB, err = AdvanceError[E](epB, suspB)
			if err == nil {
				progress = true
			}
		}
		if !progress {
			bo.Wait()
		} else {
			bo.Reset()
		}
	}
	return resultA, resultB
}

// StepError evaluates a session protocol with error support until the first
// effect suspension. Returns (Either[E, R], nil) on completion or error,
// or (zero, suspension) if pending.
func StepError[E, R any](protocol kont.Expr[R]) (kont.Either[E, R], *kont.Suspension[kont.Either[E, R]]) {
	wrapped := kont.ExprMap(protocol, func(r R) kont.Either[E, R] {
		return kont.Right[E, R](r)
	})
	return kont.StepExpr(wrapped)
}

// AdvanceError dispatches the suspended operation on the endpoint.
// Session ops are non-blocking (ErrWouldBlock). Error ops are eager:
// Throw discards the suspension and returns Left.
func AdvanceError[E, R any](ep *Endpoint, susp *kont.Suspension[kont.Either[E, R]]) (kont.Either[E, R], *kont.Suspension[kont.Either[E, R]], error) {
	// Session ops: non-blocking dispatch
	if sop, ok := susp.Op().(sessionDispatcher); ok {
		v, err := sop.DispatchSession(&ep.ctx)
		if err != nil {
			var zero kont.Either[E, R]
			return zero, susp, err
		}
		result, next := susp.Resume(v)
		return result, next, nil
	}
	// Error ops: eager dispatch
	if eop, ok := susp.Op().(interface {
		DispatchError(ctx *kont.ErrorContext[E]) (kont.Resumed, bool)
	}); ok {
		var ctx kont.ErrorContext[E]
		v, _ := eop.DispatchError(&ctx)
		if ctx.HasErr {
			susp.Discard()
			return kont.Left[E, R](ctx.Err), nil, nil
		}
		result, next := susp.Resume(v)
		return result, next, nil
	}
	panic("sess: unhandled effect in AdvanceError")
}

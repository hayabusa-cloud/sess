// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess_test

import (
	"fmt"
	"testing"

	"code.hybscloud.com/iox"
	"code.hybscloud.com/kont"
	"code.hybscloud.com/sess"
)

func requireNoSessionThrow[E any](t *testing.T, thrown *E) {
	t.Helper()
	if thrown != nil {
		t.Fatalf("expected nil session throw, got %v", *thrown)
	}
}

func requireSessionThrow[E comparable](t *testing.T, thrown *E, want E) {
	t.Helper()
	if thrown == nil {
		t.Fatal("expected non-nil session throw")
	}
	if *thrown != want {
		t.Fatalf("session throw got %v, want %v", *thrown, want)
	}
}

func TestExecErrorSuccess(t *testing.T) {
	// Success path: no error thrown, result is Right
	client := sess.SendThen(42, sess.CloseDone("ok"))
	server := sess.RecvBind(func(n int) kont.Eff[string] {
		return sess.CloseDone(fmt.Sprintf("got %d", n))
	})

	clientResult, serverResult, thrown := sess.RunError[string, string, string](client, server)
	requireNoSessionThrow(t, thrown)
	if !clientResult.IsRight() {
		t.Fatalf("client expected Right, got Left")
	}
	cv, _ := clientResult.GetRight()
	if cv != "ok" {
		t.Fatalf("client got %q, want %q", cv, "ok")
	}
	if !serverResult.IsRight() {
		t.Fatalf("server expected Right, got Left")
	}
	sv, _ := serverResult.GetRight()
	if sv != "got 42" {
		t.Fatalf("server got %q, want %q", sv, "got 42")
	}
}

func TestExecErrorThrow(t *testing.T) {
	// Throw path: client throws, result is Left
	client := sess.SendThen(42,
		kont.ThrowError[string, string]("boom"),
	)
	server := sess.RecvBind(func(n int) kont.Eff[string] {
		return sess.CloseDone(fmt.Sprintf("got %d", n))
	})

	clientResult, _, thrown := sess.RunError[string, string, string](client, server)
	requireSessionThrow(t, thrown, "boom")
	if !clientResult.IsLeft() {
		t.Fatalf("client expected Left, got Right")
	}
	errVal, _ := clientResult.GetLeft()
	if errVal != "boom" {
		t.Fatalf("client error got %q, want %q", errVal, "boom")
	}
}

func TestExecErrorCatchRecovery(t *testing.T) {
	// Catch recovery: error-only body/handler, then session ops
	// Catch body and handler must be pure error effects (no session ops).
	protocol := kont.Bind(
		kont.CatchError(
			kont.ThrowError[string, string]("fail"),
			func(e string) kont.Eff[string] {
				return kont.Pure("recovered: " + e)
			},
		),
		func(s string) kont.Eff[string] {
			return sess.SendThen(s, sess.CloseDone(s))
		},
	)

	server := sess.RecvBind(func(s string) kont.Eff[string] {
		return sess.CloseDone(s)
	})

	clientResult, serverResult, thrown := sess.RunError[string, string, string](protocol, server)
	requireNoSessionThrow(t, thrown)
	if !clientResult.IsRight() {
		t.Fatalf("client expected Right, got Left")
	}
	cv, _ := clientResult.GetRight()
	if cv != "recovered: fail" {
		t.Fatalf("client got %q, want %q", cv, "recovered: fail")
	}
	if !serverResult.IsRight() {
		t.Fatalf("server expected Right, got Left")
	}
	sv, _ := serverResult.GetRight()
	if sv != "recovered: fail" {
		t.Fatalf("server got %q, want %q", sv, "recovered: fail")
	}
}

func TestExecErrorExprSuccess(t *testing.T) {
	// Expr-world success path
	client := sess.ExprSendThen(42, sess.ExprCloseDone("ok"))
	server := sess.ExprRecvBind(func(n int) kont.Expr[string] {
		return sess.ExprCloseDone(fmt.Sprintf("got %d", n))
	})

	clientResult, serverResult, thrown := sess.RunErrorExpr[string, string, string](client, server)
	requireNoSessionThrow(t, thrown)
	if !clientResult.IsRight() {
		t.Fatalf("client expected Right, got Left")
	}
	cv, _ := clientResult.GetRight()
	if cv != "ok" {
		t.Fatalf("client got %q, want %q", cv, "ok")
	}
	if !serverResult.IsRight() {
		t.Fatalf("server expected Right, got Left")
	}
	sv, _ := serverResult.GetRight()
	if sv != "got 42" {
		t.Fatalf("server got %q, want %q", sv, "got 42")
	}
}

func TestExecErrorExprThrow(t *testing.T) {
	// Expr-world throw path
	client := sess.ExprSendThen(42,
		kont.ExprThrowError[string, string]("expr-boom"),
	)
	server := sess.ExprRecvBind(func(n int) kont.Expr[string] {
		return sess.ExprCloseDone(fmt.Sprintf("got %d", n))
	})

	clientResult, _, thrown := sess.RunErrorExpr[string, string, string](client, server)
	requireSessionThrow(t, thrown, "expr-boom")
	if !clientResult.IsLeft() {
		t.Fatalf("client expected Left, got Right")
	}
	errVal, _ := clientResult.GetLeft()
	if errVal != "expr-boom" {
		t.Fatalf("client error got %q, want %q", errVal, "expr-boom")
	}
}

func TestRunErrorExprCatchRecovery(t *testing.T) {
	client := sess.Reify(kont.Bind(
		kont.CatchError(
			kont.ThrowError[string, string]("fail"),
			func(e string) kont.Eff[string] {
				return kont.Pure("recovered: " + e)
			},
		),
		func(s string) kont.Eff[string] {
			return sess.SendThen(s, sess.CloseDone(s))
		},
	))
	server := sess.ExprRecvBind(func(s string) kont.Expr[string] {
		return sess.ExprCloseDone(s)
	})

	clientResult, serverResult, thrown := sess.RunErrorExpr[string, string, string](client, server)
	requireNoSessionThrow(t, thrown)
	if !clientResult.IsRight() {
		t.Fatalf("client expected Right, got Left")
	}
	clientValue, _ := clientResult.GetRight()
	if clientValue != "recovered: fail" {
		t.Fatalf("client got %q, want %q", clientValue, "recovered: fail")
	}
	if !serverResult.IsRight() {
		t.Fatalf("server expected Right, got Left")
	}
	serverValue, _ := serverResult.GetRight()
	if serverValue != "recovered: fail" {
		t.Fatalf("server got %q, want %q", serverValue, "recovered: fail")
	}
}

func TestRunErrorExprThrowKeepsBlockedPeerLocalResultUnresolved(t *testing.T) {
	client := kont.ExprThrowError[string, string]("boom")
	server := sess.ExprRecvBind(func(v int) kont.Expr[string] {
		return sess.ExprCloseDone(fmt.Sprintf("got %d", v))
	})

	clientResult, serverResult, thrown := sess.RunErrorExpr[string, string, string](client, server)
	requireSessionThrow(t, thrown, "boom")
	if !clientResult.IsLeft() {
		t.Fatalf("client expected Left, got Right")
	}
	clientErr, _ := clientResult.GetLeft()
	if clientErr != "boom" {
		t.Fatalf("client error got %q, want %q", clientErr, "boom")
	}
	if !serverResult.IsLeft() {
		t.Fatalf("server expected zero Left while unresolved, got Right")
	}
	serverErr, _ := serverResult.GetLeft()
	if serverErr != "" {
		t.Fatalf("server unresolved peer result got %q, want zero-value left", serverErr)
	}
}

func TestRunErrorExprThrowFromServerKeepsBlockedPeerLocalResultUnresolved(t *testing.T) {
	client := sess.ExprRecvBind(func(v int) kont.Expr[string] {
		return sess.ExprCloseDone(fmt.Sprintf("got %d", v))
	})
	server := kont.ExprThrowError[string, string]("boom")

	clientResult, serverResult, thrown := sess.RunErrorExpr[string, string, string](client, server)
	requireSessionThrow(t, thrown, "boom")
	if !serverResult.IsLeft() {
		t.Fatalf("server expected Left, got Right")
	}
	serverErr, _ := serverResult.GetLeft()
	if serverErr != "boom" {
		t.Fatalf("server error got %q, want %q", serverErr, "boom")
	}
	if !clientResult.IsLeft() {
		t.Fatalf("client expected zero Left while unresolved, got Right")
	}
	clientErr, _ := clientResult.GetLeft()
	if clientErr != "" {
		t.Fatalf("client unresolved peer result got %q, want zero-value left", clientErr)
	}
}

func TestRunErrorExprBothSidesThrowUsesFirstObservedSessionThrow(t *testing.T) {
	client := kont.ExprThrowError[string, string]("left-boom")
	server := kont.ExprThrowError[string, string]("right-boom")

	clientResult, serverResult, thrown := sess.RunErrorExpr[string, string, string](client, server)
	requireSessionThrow(t, thrown, "left-boom")
	if !clientResult.IsLeft() {
		t.Fatalf("client expected Left, got Right")
	}
	clientErr, _ := clientResult.GetLeft()
	if clientErr != "left-boom" {
		t.Fatalf("client error got %q, want %q", clientErr, "left-boom")
	}
	if !serverResult.IsLeft() {
		t.Fatalf("server expected zero Left while unresolved, got Right")
	}
	serverErr, _ := serverResult.GetLeft()
	if serverErr != "" {
		t.Fatalf("server unresolved peer result got %q, want zero-value left", serverErr)
	}
}

func TestRunErrorExprRetriesBlockedRecvUntilPeerSendSucceeds(t *testing.T) {
	client := sess.ExprRecvBind(func(n int) kont.Expr[string] {
		return sess.ExprCloseDone(fmt.Sprintf("got %d", n))
	})
	server := sess.ExprSendThen(42, sess.ExprCloseDone("peer"))

	clientResult, serverResult, thrown := sess.RunErrorExpr[string, string, string](client, server)
	requireNoSessionThrow(t, thrown)
	if !clientResult.IsRight() {
		t.Fatalf("client expected Right, got Left")
	}
	clientValue, _ := clientResult.GetRight()
	if clientValue != "got 42" {
		t.Fatalf("client got %q, want %q", clientValue, "got 42")
	}
	if !serverResult.IsRight() {
		t.Fatalf("server expected Right, got Left")
	}
	serverValue, _ := serverResult.GetRight()
	if serverValue != "peer" {
		t.Fatalf("server got %q, want %q", serverValue, "peer")
	}
}

func TestRunErrorExprRetriesBlockedRecvBeforeThrow(t *testing.T) {
	client := sess.ExprRecvBind(func(n int) kont.Expr[string] {
		return kont.ExprThrowError[string, string](fmt.Sprintf("boom %d", n))
	})
	server := sess.ExprSendThen(42, sess.ExprCloseDone("peer"))

	clientResult, serverResult, thrown := sess.RunErrorExpr[string, string, string](client, server)
	requireSessionThrow(t, thrown, "boom 42")
	if !clientResult.IsLeft() {
		t.Fatalf("client expected Left, got Right")
	}
	clientErr, _ := clientResult.GetLeft()
	if clientErr != "boom 42" {
		t.Fatalf("client error got %q, want %q", clientErr, "boom 42")
	}
	if !serverResult.IsRight() {
		t.Fatalf("server expected Right, got Left")
	}
	serverValue, _ := serverResult.GetRight()
	if serverValue != "peer" {
		t.Fatalf("server got %q, want %q", serverValue, "peer")
	}
}

func TestStepErrorSuccess(t *testing.T) {
	sender, receiver := sess.New()
	_, send := sess.StepError[string](sess.ExprSendThen(42, kont.ExprReturn("ok")))
	_, recv := sess.StepError[string](sess.ExprRecvBind(func(n int) kont.Expr[string] {
		return kont.ExprReturn(fmt.Sprintf("got %d", n))
	}))
	if send == nil || recv == nil {
		t.Fatal("expected pending send and receive")
	}
	sent, next, err := sess.AdvanceError[string](sender, send)
	if got, ok := sent.GetRight(); !ok || got != "ok" || next != nil || err != nil {
		t.Fatalf("send returned (%v, %p, %v), want Right(ok) and completion", sent, next, err)
	}
	received, next, err := sess.AdvanceError[string](receiver, recv)
	if got, ok := received.GetRight(); !ok || got != "got 42" || next != nil || err != nil {
		t.Fatalf("receive returned (%v, %p, %v), want Right(got 42) and completion", received, next, err)
	}
}

func TestExecErrorExprThrowAfterSend(t *testing.T) {
	protocol := sess.ExprSendThen(1,
		kont.ExprThrowError[string, string]("step-boom"),
	)

	epA, _ := sess.New()
	result := sess.ExecErrorExpr[string](epA, protocol)
	if !result.IsLeft() {
		t.Fatalf("expected Left, got Right")
	}
	errVal, _ := result.GetLeft()
	if errVal != "step-boom" {
		t.Fatalf("error got %q, want %q", errVal, "step-boom")
	}
}

func TestStepErrorThrow(t *testing.T) {
	const cause = "step-boom"
	protocol := kont.ExprMap(kont.ExprThrowError[string, int](cause), func(n int) int {
		t.Fatal("resumed past Throw")
		return n
	})
	_, pending := sess.StepError[string](protocol)
	if pending == nil {
		t.Fatal("StepError completed before yielding Throw")
	}
	if op, ok := pending.Op().(kont.Throw[string]); !ok || op.Err != cause {
		t.Fatalf("StepError yielded %v, want Throw(%q)", pending.Op(), cause)
	}
	ep, _ := sess.New()
	result, next, err := sess.AdvanceError[string](ep, pending)
	if msg, ok := result.GetLeft(); !ok || msg != cause || next != nil || err != nil {
		t.Fatalf("AdvanceError got (%v, %v, %v), want Left(%q) and completion", result, next, err, cause)
	}
}

func TestAdvanceErrorWouldBlock(t *testing.T) {
	sender, receiver := sess.New()
	_, pending := sess.StepError[string](kont.ExprPerform(sess.Recv[int]{}))
	_, retry, err := sess.AdvanceError[string](receiver, pending)
	if !iox.IsWouldBlock(err) || retry != pending {
		t.Fatalf("empty receive returned (%p, %v), want original suspension and backpressure", retry, err)
	}
	advanceOne(t, sender, kont.ExprPerform(sess.Send[int]{Value: 99}))
	result, next, err := sess.AdvanceError[string](receiver, retry)
	if got, ok := result.GetRight(); !ok || got != 99 || next != nil || err != nil {
		t.Fatalf("receive retry returned (%v, %p, %v), want Right(99) and completion", result, next, err)
	}
}

func TestAdvanceErrorUnhandledPanics(t *testing.T) {
	// AdvanceError with bogus operation panics
	type bogus struct{ kont.Phantom[int] }

	protocol := kont.ExprPerform(bogus{})
	wrapped := kont.ExprMap(protocol, func(n int) kont.Either[string, int] {
		return kont.Right[string, int](n)
	})

	_, susp := kont.StepExpr(wrapped)
	if susp == nil {
		t.Fatal("expected suspension")
	}

	ep, _ := sess.New()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for unhandled effect")
		}
		msg, ok := r.(string)
		if !ok || msg != "sess: unhandled effect in AdvanceError" {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()
	sess.AdvanceError[string](ep, susp)
}

func TestExecErrorDispatchUnhandledPanics(t *testing.T) {
	// ExecError with bogus operation panics (sessionErrorHandler.Dispatch)
	type bogus struct{ kont.Phantom[int] }

	ep, _ := sess.New()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for unhandled effect")
		}
		msg, ok := r.(string)
		if !ok || msg != "sess: unhandled effect in SessionErrorHandler" {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()
	sess.ExecError[string](ep, kont.Perform(bogus{}))
}

func TestLoopWithError(t *testing.T) {
	// Combined Loop + Error: loop sends values, throws when reaching a limit
	client := sess.Loop(0, func(i int) kont.Eff[kont.Either[int, string]] {
		if i >= 3 {
			return kont.ThrowError[string, kont.Either[int, string]]("limit")
		}
		return sess.SendThen(i, kont.Pure(kont.Left[int, string](i+1)))
	})

	server := sess.RecvBind(func(a int) kont.Eff[int] {
		return sess.RecvBind(func(b int) kont.Eff[int] {
			return sess.RecvBind(func(c int) kont.Eff[int] {
				return sess.CloseDone(a + b + c)
			})
		})
	})

	clientResult, _, thrown := sess.RunError[string, string, int](client, server)
	requireSessionThrow(t, thrown, "limit")
	if !clientResult.IsLeft() {
		t.Fatalf("client expected Left, got Right")
	}
	errVal, _ := clientResult.GetLeft()
	if errVal != "limit" {
		t.Fatalf("client error got %q, want %q", errVal, "limit")
	}
}

func TestExprLoopWithError(t *testing.T) {
	// Combined ExprLoop + Error: loop sends values, throws when reaching a limit
	client := sess.ExprLoop(0, func(i int) kont.Expr[kont.Either[int, string]] {
		if i >= 3 {
			return kont.ExprThrowError[string, kont.Either[int, string]]("limit")
		}
		return sess.ExprSendThen(i, kont.ExprReturn(kont.Left[int, string](i+1)))
	})

	server := sess.ExprRecvBind(func(a int) kont.Expr[int] {
		return sess.ExprRecvBind(func(b int) kont.Expr[int] {
			return sess.ExprRecvBind(func(c int) kont.Expr[int] {
				return sess.ExprCloseDone(a + b + c)
			})
		})
	})

	clientResult, _, thrown := sess.RunErrorExpr[string, string, int](client, server)
	requireSessionThrow(t, thrown, "limit")
	if !clientResult.IsLeft() {
		t.Fatalf("client expected Left, got Right")
	}
	errVal, _ := clientResult.GetLeft()
	if errVal != "limit" {
		t.Fatalf("client error got %q, want %q", errVal, "limit")
	}
}

func TestExecErrorSingleEndpoint(t *testing.T) {
	skipRace(t)
	// execError on a single endpoint
	epA, epB := sess.New()

	done := make(chan struct{})
	go func() {
		sess.Exec(epB, sess.RecvBind(func(n int) kont.Eff[string] {
			return sess.CloseDone(fmt.Sprintf("got %d", n))
		}))
		close(done)
	}()

	result := sess.ExecError[string](epA, sess.SendThen(7, sess.CloseDone("ok")))
	<-done

	if !result.IsRight() {
		t.Fatalf("expected Right, got Left")
	}
	rv, _ := result.GetRight()
	if rv != "ok" {
		t.Fatalf("got %q, want %q", rv, "ok")
	}
}

func TestExecErrorExprSingleEndpoint(t *testing.T) {
	skipRace(t)
	// execErrorExpr on a single endpoint
	epA, epB := sess.New()

	done := make(chan struct{})
	go func() {
		execExpr(epB, sess.ExprRecvBind(func(n int) kont.Expr[string] {
			return sess.ExprCloseDone(fmt.Sprintf("got %d", n))
		}))
		close(done)
	}()

	result := sess.ExecErrorExpr[string](epA, sess.ExprSendThen(7, sess.ExprCloseDone("ok")))
	<-done

	if !result.IsRight() {
		t.Fatalf("expected Right, got Left")
	}
	rv, _ := result.GetRight()
	if rv != "ok" {
		t.Fatalf("got %q, want %q", rv, "ok")
	}
}

func TestExecErrorCatchSuccess(t *testing.T) {
	skipRace(t)
	// ExecError with Catch that succeeds (body doesn't throw).
	// Exercises the non-throw error dispatch path in Dispatch (error.go:33).
	epA, epB := sess.New()

	done := make(chan struct{})
	go func() {
		sess.Exec(epB, sess.RecvBind(func(s string) kont.Eff[string] {
			return sess.CloseDone(s)
		}))
		close(done)
	}()

	body := kont.Pure[string]("ok")
	caught := kont.CatchError[string](body, func(e string) kont.Eff[string] {
		return kont.Pure("caught: " + e)
	})
	protocol := kont.Bind(caught, func(s string) kont.Eff[string] {
		return sess.SendThen(s, sess.CloseDone(s))
	})

	result := sess.ExecError[string](epA, protocol)
	<-done

	if !result.IsRight() {
		t.Fatalf("expected Right, got Left")
	}
	rv, _ := result.GetRight()
	if rv != "ok" {
		t.Fatalf("got %q, want %q", rv, "ok")
	}
}

func TestStepErrorPureReturn(t *testing.T) {
	result, susp := sess.StepError[string, int](kont.ExprReturn(42))
	if susp != nil {
		t.Fatal("expected immediate completion, got suspension")
	}
	if !result.IsRight() {
		t.Fatalf("expected Right, got Left")
	}
	rv, _ := result.GetRight()
	if rv != 42 {
		t.Fatalf("got %d, want 42", rv)
	}
}

func TestErrorRunnersPreserveFrameInput(t *testing.T) {
	addOne := &kont.MapFrame[any, any]{
		F:    func(v any) any { return v.(int) + 1 },
		Next: kont.ReturnFrame{},
	}
	m := kont.Expr[int]{Value: 41, Frame: addOne}

	ep, _ := sess.New()
	if n, ok := sess.ExecErrorExpr[string](ep, m).GetRight(); !ok || n != 42 {
		t.Fatalf("ExecErrorExpr got (%d, %t), want Right(42)", n, ok)
	}
	result, pending := sess.StepError[string](m)
	if n, ok := result.GetRight(); !ok || n != 42 || pending != nil {
		t.Fatalf("StepError got (%d, %t, %v), want Right(42) and completion", n, ok, pending)
	}
	left, right, thrown := sess.RunErrorExpr[string](m, kont.ExprReturn("peer"))
	n, ok := left.GetRight()
	peer, peerOK := right.GetRight()
	if thrown != nil || !ok || n != 42 || !peerOK || peer != "peer" {
		t.Fatalf("RunErrorExpr got (%v, %v, %v), want Right(42), Right(peer) and no throw", left, right, thrown)
	}
}

func TestErrorRunnersNormalNil(t *testing.T) {
	ep, _ := sess.New()
	m := kont.Suspend(func(func(int) kont.Resumed) kont.Resumed { return nil })
	if n, ok := sess.ExecError[string](ep, m).GetRight(); !ok || n != 0 {
		t.Fatalf("ExecError got (%d, %t), want Right(0)", n, ok)
	}
	if err, ok := sess.ExecErrorExpr[string](ep, sess.ExprCloseDone[error](nil)).GetRight(); !ok || err != nil {
		t.Fatalf("ExecErrorExpr got (%v, %t), want Right(nil)", err, ok)
	}
	frame := &kont.MapFrame[any, any]{F: func(any) any { return nil }, Next: kont.ReturnFrame{}}
	result, pending := sess.StepError[string](kont.Expr[int]{Value: 1, Frame: frame})
	if n, ok := result.GetRight(); !ok || n != 0 || pending != nil {
		t.Fatalf("StepError got (%d, %t, %v), want Right(0) and completion", n, ok, pending)
	}
}

func TestErrorRunnersNilThrow(t *testing.T) {
	ep, _ := sess.New()
	result := sess.ExecError[error](ep, kont.ThrowError[error, int](nil))
	if err, ok := result.GetLeft(); !ok || err != nil {
		t.Fatalf("ExecError got (%v, %t), want Left(nil)", err, ok)
	}
	left, _, thrown := sess.RunErrorExpr[error](kont.ExprThrowError[error, int](nil), kont.ExprReturn("peer"))
	if err, ok := left.GetLeft(); !ok || err != nil || thrown == nil || *thrown != nil {
		t.Fatalf("RunErrorExpr got (%v, %v), want Left(nil) and a reported nil throw", left, thrown)
	}
}

func TestAdvanceErrorCatchStepping(t *testing.T) {
	// Stepping through Catch that succeeds — non-throw error dispatch in AdvanceError
	body := kont.Pure[string]("ok")
	caught := kont.CatchError[string](body, func(e string) kont.Eff[string] {
		return kont.Pure("caught: " + e)
	})
	protocol := sess.Reify(caught) // Cont → Expr for stepping

	result, susp := sess.StepError[string, string](protocol)
	if susp == nil {
		t.Fatalf("expected suspension for Catch, got result %v", result)
	}

	ep, _ := sess.New()
	result, susp, err := sess.AdvanceError[string](ep, susp)
	if err != nil {
		t.Fatalf("AdvanceError error: %v", err)
	}
	// Catch succeeded (body didn't throw), should get Right("ok")
	for susp != nil {
		result, susp, err = sess.AdvanceError[string](ep, susp)
		if err != nil {
			t.Fatalf("AdvanceError error: %v", err)
		}
	}
	if !result.IsRight() {
		t.Fatalf("expected Right, got Left")
	}
	rv, _ := result.GetRight()
	if rv != "ok" {
		t.Fatalf("got %q, want %q", rv, "ok")
	}
}

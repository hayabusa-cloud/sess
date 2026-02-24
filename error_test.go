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

func TestExecErrorSuccess(t *testing.T) {
	skipRace(t)
	// Success path: no error thrown, result is Right
	client := sess.SendThen(42, sess.CloseDone("ok"))
	server := sess.RecvBind(func(n int) kont.Eff[string] {
		return sess.CloseDone(fmt.Sprintf("got %d", n))
	})

	clientResult, serverResult := sess.RunError[string, string, string](client, server)
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
	skipRace(t)
	// Throw path: client throws, result is Left
	client := sess.SendThen(42,
		kont.ThrowError[string, string]("boom"),
	)
	server := sess.RecvBind(func(n int) kont.Eff[string] {
		return sess.CloseDone(fmt.Sprintf("got %d", n))
	})

	clientResult, _ := sess.RunError[string, string, string](client, server)
	if !clientResult.IsLeft() {
		t.Fatalf("client expected Left, got Right")
	}
	errVal, _ := clientResult.GetLeft()
	if errVal != "boom" {
		t.Fatalf("client error got %q, want %q", errVal, "boom")
	}
}

func TestExecErrorCatchRecovery(t *testing.T) {
	skipRace(t)
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

	clientResult, serverResult := sess.RunError[string, string, string](protocol, server)
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
	skipRace(t)
	// Expr-world success path
	client := sess.ExprSendThen(42, sess.ExprCloseDone("ok"))
	server := sess.ExprRecvBind(func(n int) kont.Expr[string] {
		return sess.ExprCloseDone(fmt.Sprintf("got %d", n))
	})

	clientResult, serverResult := sess.RunErrorExpr[string, string, string](client, server)
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
	skipRace(t)
	// Expr-world throw path
	client := sess.ExprSendThen(42,
		kont.ExprThrowError[string, string]("expr-boom"),
	)
	server := sess.ExprRecvBind(func(n int) kont.Expr[string] {
		return sess.ExprCloseDone(fmt.Sprintf("got %d", n))
	})

	clientResult, _ := sess.RunErrorExpr[string, string, string](client, server)
	if !clientResult.IsLeft() {
		t.Fatalf("client expected Left, got Right")
	}
	errVal, _ := clientResult.GetLeft()
	if errVal != "expr-boom" {
		t.Fatalf("client error got %q, want %q", errVal, "expr-boom")
	}
}

func TestStepErrorSuccess(t *testing.T) {
	skipRace(t)
	// Stepping with errors: success path
	client := sess.ExprSendThen(42, sess.ExprCloseDone("ok"))
	server := sess.ExprRecvBind(func(n int) kont.Expr[string] {
		return sess.ExprCloseDone(fmt.Sprintf("got %d", n))
	})

	epA, epB := sess.New()

	var clientResult kont.Either[string, string]
	done := make(chan struct{})
	go func() {
		clientResult = sess.ExecErrorExpr[string](epA, client)
		close(done)
	}()
	serverResult := sess.ExecErrorExpr[string](epB, server)
	<-done

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

func TestStepErrorThrow(t *testing.T) {
	skipRace(t)
	// Stepping with errors: throw path
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

func TestAdvanceErrorWouldBlock(t *testing.T) {
	skipRace(t)
	// AdvanceError returns ErrWouldBlock when queue is empty
	protocol := sess.ExprRecvBind(func(n int) kont.Expr[int] {
		return sess.ExprCloseDone(n)
	})

	result, susp := sess.StepError[string, int](protocol)
	if susp == nil {
		t.Fatalf("expected suspension, got result %v", result)
	}

	epA, epB := sess.New()

	// epA's recvQ is empty — should get ErrWouldBlock
	_, retrySusp, err := sess.AdvanceError[string](epA, susp)
	if !iox.IsWouldBlock(err) {
		t.Fatalf("expected ErrWouldBlock, got %v", err)
	}
	if retrySusp != susp {
		t.Fatal("suspension should be returned unconsumed on error")
	}

	// Enqueue from peer side, then retry
	sender := sess.ExprSendThen(99, sess.ExprCloseDone[struct{}](struct{}{}))
	peerDone := make(chan struct{})
	go func() {
		execExpr(epB, sender)
		close(peerDone)
	}()

	// Spin-retry AdvanceError until success
	for {
		result, susp, err = sess.AdvanceError[string](epA, susp)
		if err == nil {
			break
		}
	}
	<-peerDone

	// Drive remaining suspensions
	for susp != nil {
		result, susp, err = sess.AdvanceError[string](epA, susp)
		if err != nil {
			continue
		}
	}
	if !result.IsRight() {
		t.Fatalf("expected Right, got Left")
	}
	rv, _ := result.GetRight()
	if rv != 99 {
		t.Fatalf("result got %d, want 99", rv)
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
	skipRace(t)
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

	clientResult, _ := sess.RunError[string, string, int](client, server)
	if !clientResult.IsLeft() {
		t.Fatalf("client expected Left, got Right")
	}
	errVal, _ := clientResult.GetLeft()
	if errVal != "limit" {
		t.Fatalf("client error got %q, want %q", errVal, "limit")
	}
}

func TestExprLoopWithError(t *testing.T) {
	skipRace(t)
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

	clientResult, _ := sess.RunErrorExpr[string, string, int](client, server)
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

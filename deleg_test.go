// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess_test

import (
	"testing"

	"code.hybscloud.com/kont"
	"code.hybscloud.com/sess"
)

func TestDelegAcceptRoundtrip(t *testing.T) {
	skipRace(t)
	// A delegates a sub-session endpoint to B.
	// B uses the delegated endpoint to communicate with C.
	subA, subB := sess.New()

	// C: receives on subB, closes (separate goroutine — three-party protocol)
	done := make(chan string)
	go func() {
		result := sess.Exec(subB, sess.RecvBind(func(s string) kont.Eff[string] {
			return sess.CloseDone(s)
		}))
		done <- result
	}()

	// A delegates subA to B, then closes
	delegator := sess.SendThen(subA, sess.CloseDone("delegated"))

	// B accepts the endpoint, sends "hello" on it, then closes
	acceptor := sess.RecvBind(func(ep *sess.Endpoint) kont.Eff[string] {
		sess.Exec(ep, sess.SendThen("hello", sess.CloseDone("sent")))
		return sess.CloseDone("accepted")
	})

	aResult, bResult := sess.Run[string, string](delegator, acceptor)
	cResult := <-done

	if aResult != "delegated" {
		t.Fatalf("A got %q, want %q", aResult, "delegated")
	}
	if bResult != "accepted" {
		t.Fatalf("B got %q, want %q", bResult, "accepted")
	}
	if cResult != "hello" {
		t.Fatalf("C got %q, want %q", cResult, "hello")
	}
}

func TestDelegThreePartyChain(t *testing.T) {
	skipRace(t)
	// A delegates to B, B uses the delegated endpoint to talk to C
	// A ─(deleg)→ B ─(via delegated ep)→ C
	subA, subC := sess.New()

	// C: receives int, sends back doubled, closes
	cDone := make(chan int)
	go func() {
		result := sess.Exec(subC, sess.RecvBind(func(n int) kont.Eff[int] {
			return sess.SendThen(n*2, sess.CloseDone(n))
		}))
		cDone <- result
	}()

	// A: delegates subA, then closes
	delegator := sess.SendThen(subA, sess.CloseDone("done"))

	// B: accepts endpoint, sends 21 on it, receives doubled, closes
	acceptor := sess.RecvBind(func(ep *sess.Endpoint) kont.Eff[int] {
		result := sess.Exec(ep, sess.SendThen(21,
			sess.RecvBind(func(doubled int) kont.Eff[int] {
				return sess.CloseDone(doubled)
			}),
		))
		return sess.CloseDone(result)
	})

	aResult, bResult := sess.Run[string, int](delegator, acceptor)
	cResult := <-cDone

	if aResult != "done" {
		t.Fatalf("A got %q, want %q", aResult, "done")
	}
	if bResult != 42 {
		t.Fatalf("B got %d, want 42", bResult)
	}
	if cResult != 21 {
		t.Fatalf("C got %d, want 21", cResult)
	}
}

func TestExprDelegAcceptRoundtrip(t *testing.T) {
	skipRace(t)
	// Expr-world delegation roundtrip
	subA, subB := sess.New()

	done := make(chan string)
	go func() {
		result := sess.ExecExpr(subB, sess.ExprRecvBind(func(s string) kont.Expr[string] {
			return sess.ExprCloseDone(s)
		}))
		done <- result
	}()

	delegator := sess.ExprSendThen(subA, sess.ExprCloseDone("delegated"))

	acceptor := sess.ExprRecvBind(func(ep *sess.Endpoint) kont.Expr[string] {
		sess.ExecExpr(ep, sess.ExprSendThen("hello", sess.ExprCloseDone[struct{}](struct{}{})))
		return sess.ExprCloseDone("accepted")
	})

	aResult, bResult := sess.RunExpr[string, string](delegator, acceptor)
	cResult := <-done

	if aResult != "delegated" {
		t.Fatalf("A got %q, want %q", aResult, "delegated")
	}
	if bResult != "accepted" {
		t.Fatalf("B got %q, want %q", bResult, "accepted")
	}
	if cResult != "hello" {
		t.Fatalf("C got %q, want %q", cResult, "hello")
	}
}

func TestDelegStepping(t *testing.T) {
	skipRace(t)
	// Step through delegation via manual Step+Advance
	subA, subB := sess.New()

	// C receives on subB
	cDone := make(chan int)
	go func() {
		result := sess.ExecExpr(subB, sess.ExprRecvBind(func(n int) kont.Expr[int] {
			return sess.ExprCloseDone(n)
		}))
		cDone <- result
	}()

	epA, epB := sess.New()

	// Step both sides — should suspend on Send/Recv
	delegator := sess.ExprSendThen(subA, sess.ExprCloseDone("deleg"))
	resultA, suspA := sess.Step[string](delegator)
	if suspA == nil {
		t.Fatalf("expected suspension on Send, got %v", resultA)
	}

	acceptor := sess.ExprRecvBind(func(ep *sess.Endpoint) kont.Expr[string] {
		sess.ExecExpr(ep, sess.ExprSendThen(99, sess.ExprCloseDone[struct{}](struct{}{})))
		return sess.ExprCloseDone("accepted")
	})
	resultB, suspB := sess.Step[string](acceptor)
	if suspB == nil {
		t.Fatalf("expected suspension on Recv, got %v", resultB)
	}

	// Advance both sides manually
	for suspA != nil || suspB != nil {
		if suspA != nil {
			var err error
			resultA, suspA, err = sess.Advance(epA, suspA)
			if err != nil {
				continue
			}
		}
		if suspB != nil {
			var err error
			resultB, suspB, err = sess.Advance(epB, suspB)
			if err != nil {
				continue
			}
		}
	}
	cResult := <-cDone

	if resultA != "deleg" {
		t.Fatalf("A got %q, want %q", resultA, "deleg")
	}
	if resultB != "accepted" {
		t.Fatalf("B got %q, want %q", resultB, "accepted")
	}
	if cResult != 99 {
		t.Fatalf("C got %d, want 99", cResult)
	}
}

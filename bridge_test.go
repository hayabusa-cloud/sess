// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess_test

import (
	"fmt"
	"testing"

	"code.hybscloud.com/kont"
	"code.hybscloud.com/sess"
)

func TestReifyContToExpr(t *testing.T) {
	skipRace(t)
	// Cont protocol → Reify → execExpr
	cont := sess.SendThen(42,
		sess.RecvBind(func(s string) kont.Eff[string] {
			return sess.CloseDone(s)
		}),
	)
	expr := sess.Reify(cont)

	server := sess.ExprRecvBind(func(n int) kont.Expr[string] {
		return sess.ExprSendThen(fmt.Sprintf("got %d", n),
			sess.ExprCloseDone("done"),
		)
	})

	clientResult, serverResult := sess.RunExpr[string, string](expr, server)
	if clientResult != "got 42" {
		t.Fatalf("client got %q, want %q", clientResult, "got 42")
	}
	if serverResult != "done" {
		t.Fatalf("server got %q, want %q", serverResult, "done")
	}
}

func TestReflectExprToCont(t *testing.T) {
	skipRace(t)
	// Expr protocol → Reflect → exec
	expr := sess.ExprSendThen(42,
		sess.ExprRecvBind(func(s string) kont.Expr[string] {
			return sess.ExprCloseDone(s)
		}),
	)
	cont := sess.Reflect(expr)

	server := sess.RecvBind(func(n int) kont.Eff[string] {
		return sess.SendThen(fmt.Sprintf("got %d", n),
			sess.CloseDone("done"),
		)
	})

	clientResult, serverResult := sess.Run[string, string](cont, server)
	if clientResult != "got 42" {
		t.Fatalf("client got %q, want %q", clientResult, "got 42")
	}
	if serverResult != "done" {
		t.Fatalf("server got %q, want %q", serverResult, "done")
	}
}

func TestRoundTripReifyReflect(t *testing.T) {
	skipRace(t)
	// Reflect(Reify(cont)) preserves semantics
	cont := sess.SendThen(7,
		sess.RecvBind(func(n int) kont.Eff[int] {
			return sess.CloseDone(n)
		}),
	)
	roundTripped := sess.Reflect(sess.Reify(cont))

	server := sess.RecvBind(func(n int) kont.Eff[int] {
		return sess.SendThen(n*3, sess.CloseDone(n*3))
	})

	clientResult, serverResult := sess.Run[int, int](roundTripped, server)
	if clientResult != 21 {
		t.Fatalf("client got %d, want 21", clientResult)
	}
	if serverResult != 21 {
		t.Fatalf("server got %d, want 21", serverResult)
	}
}

func TestRoundTripReflectReify(t *testing.T) {
	skipRace(t)
	// Reify(Reflect(expr)) preserves semantics
	expr := sess.ExprSendThen(5,
		sess.ExprRecvBind(func(n int) kont.Expr[int] {
			return sess.ExprCloseDone(n)
		}),
	)
	roundTripped := sess.Reify(sess.Reflect(expr))

	server := sess.ExprRecvBind(func(n int) kont.Expr[int] {
		return sess.ExprSendThen(n*4, sess.ExprCloseDone(n*4))
	})

	clientResult, serverResult := sess.RunExpr[int, int](roundTripped, server)
	if clientResult != 20 {
		t.Fatalf("client got %d, want 20", clientResult)
	}
	if serverResult != 20 {
		t.Fatalf("server got %d, want 20", serverResult)
	}
}

func TestBridgeSelectOffer(t *testing.T) {
	skipRace(t)
	// Branch protocols survive Cont→Expr conversion
	cont := sess.SelectLThen(
		sess.SendThen(33, sess.CloseDone("left")),
	)
	expr := sess.Reify(cont)

	server := sess.ExprOfferBranch(
		func() kont.Expr[string] {
			return sess.ExprRecvBind(func(n int) kont.Expr[string] {
				return sess.ExprCloseDone(fmt.Sprintf("left:%d", n))
			})
		},
		func() kont.Expr[string] {
			return sess.ExprCloseDone("right")
		},
	)

	clientResult, serverResult := sess.RunExpr[string, string](expr, server)
	if clientResult != "left" {
		t.Fatalf("client got %q, want %q", clientResult, "left")
	}
	if serverResult != "left:33" {
		t.Fatalf("server got %q, want %q", serverResult, "left:33")
	}
}

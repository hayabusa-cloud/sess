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

func TestExprSendRecv(t *testing.T) {
	skipRace(t)
	// !int.?string.end ↔ ?int.!string.end
	client := sess.ExprSendThen(42,
		sess.ExprRecvBind(func(s string) kont.Expr[string] {
			return sess.ExprCloseDone(s)
		}),
	)

	server := sess.ExprRecvBind(func(n int) kont.Expr[string] {
		return sess.ExprSendThen(fmt.Sprintf("got %d", n),
			sess.ExprCloseDone("done"),
		)
	})

	clientResult, serverResult := sess.RunExpr[string, string](client, server)
	if clientResult != "got 42" {
		t.Fatalf("client got %q, want %q", clientResult, "got 42")
	}
	if serverResult != "done" {
		t.Fatalf("server got %q, want %q", serverResult, "done")
	}
}

func TestExprSendRecvMultiple(t *testing.T) {
	skipRace(t)
	// !int.!int.?int.end ↔ ?int.?int.!int.end
	client := sess.ExprSendThen(10,
		sess.ExprSendThen(20,
			sess.ExprRecvBind(func(sum int) kont.Expr[int] {
				return sess.ExprCloseDone(sum)
			}),
		),
	)

	server := sess.ExprRecvBind(func(a int) kont.Expr[int] {
		return sess.ExprRecvBind(func(b int) kont.Expr[int] {
			return sess.ExprSendThen(a+b, sess.ExprCloseDone(a+b))
		})
	})

	clientResult, serverResult := sess.RunExpr[int, int](client, server)
	if clientResult != 30 {
		t.Fatalf("client got %d, want 30", clientResult)
	}
	if serverResult != 30 {
		t.Fatalf("server got %d, want 30", serverResult)
	}
}

func TestExprSelectOfferLeft(t *testing.T) {
	skipRace(t)
	// SelectL.!int.end ↔ Offer.?int.end
	client := sess.ExprSelectLThen(
		sess.ExprSendThen(99, sess.ExprCloseDone("left")),
	)

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

	clientResult, serverResult := sess.RunExpr[string, string](client, server)
	if clientResult != "left" {
		t.Fatalf("client got %q, want %q", clientResult, "left")
	}
	if serverResult != "left:99" {
		t.Fatalf("server got %q, want %q", serverResult, "left:99")
	}
}

func TestExprSelectOfferRight(t *testing.T) {
	skipRace(t)
	// SelectR.!string.end ↔ Offer.?string.end
	client := sess.ExprSelectRThen(
		sess.ExprSendThen("hello", sess.ExprCloseDone("right")),
	)

	server := sess.ExprOfferBranch(
		func() kont.Expr[string] {
			return sess.ExprCloseDone("left")
		},
		func() kont.Expr[string] {
			return sess.ExprRecvBind(func(s string) kont.Expr[string] {
				return sess.ExprCloseDone(fmt.Sprintf("right:%s", s))
			})
		},
	)

	clientResult, serverResult := sess.RunExpr[string, string](client, server)
	if clientResult != "right" {
		t.Fatalf("client got %q, want %q", clientResult, "right")
	}
	if serverResult != "right:hello" {
		t.Fatalf("server got %q, want %q", serverResult, "right:hello")
	}
}

func TestExprCloseOnly(t *testing.T) {
	skipRace(t)
	// end ↔ end
	a := sess.ExprCloseDone("a")
	b := sess.ExprCloseDone("b")

	resultA, resultB := sess.RunExpr[string, string](a, b)
	if resultA != "a" {
		t.Fatalf("a got %q, want %q", resultA, "a")
	}
	if resultB != "b" {
		t.Fatalf("b got %q, want %q", resultB, "b")
	}
}

func TestExprFusedProtocol(t *testing.T) {
	skipRace(t)
	// Full protocol using only Expr fused API
	client := sess.ExprSendThen(100,
		sess.ExprSendThen("hello",
			sess.ExprRecvBind(func(n int) kont.Expr[int] {
				return sess.ExprCloseDone(n)
			}),
		),
	)

	server := sess.ExprRecvBind(func(n int) kont.Expr[string] {
		return sess.ExprRecvBind(func(s string) kont.Expr[string] {
			return sess.ExprSendThen(n*2,
				sess.ExprCloseDone(fmt.Sprintf("%s:%d", s, n)),
			)
		})
	})

	clientResult, serverResult := sess.RunExpr[int, string](client, server)
	if clientResult != 200 {
		t.Fatalf("client got %d, want 200", clientResult)
	}
	if serverResult != "hello:100" {
		t.Fatalf("server got %q, want %q", serverResult, "hello:100")
	}
}

func TestExprSelectOfferReverse(t *testing.T) {
	skipRace(t)
	// Server selects, client offers — exercises epB.signal and epA.await
	server := sess.ExprSelectLThen(
		sess.ExprSendThen(77, sess.ExprCloseDone("selected")),
	)

	client := sess.ExprOfferBranch(
		func() kont.Expr[string] {
			return sess.ExprRecvBind(func(n int) kont.Expr[string] {
				return sess.ExprCloseDone(fmt.Sprintf("got %d", n))
			})
		},
		func() kont.Expr[string] {
			return sess.ExprCloseDone("right")
		},
	)

	clientResult, serverResult := sess.RunExpr[string, string](client, server)
	if clientResult != "got 77" {
		t.Fatalf("client got %q, want %q", clientResult, "got 77")
	}
	if serverResult != "selected" {
		t.Fatalf("server got %q, want %q", serverResult, "selected")
	}
}

func TestExprBidirectional(t *testing.T) {
	skipRace(t)
	// !int.?string.!bool.end ↔ ?int.!string.?bool.end
	client := sess.ExprSendThen(7,
		sess.ExprRecvBind(func(s string) kont.Expr[string] {
			return sess.ExprSendThen(true, sess.ExprCloseDone(s))
		}),
	)

	server := sess.ExprRecvBind(func(n int) kont.Expr[bool] {
		return sess.ExprSendThen(fmt.Sprintf("n=%d", n),
			sess.ExprRecvBind(func(b bool) kont.Expr[bool] {
				return sess.ExprCloseDone(b)
			}),
		)
	})

	clientResult, serverResult := sess.RunExpr[string, bool](client, server)
	if clientResult != "n=7" {
		t.Fatalf("client got %q, want %q", clientResult, "n=7")
	}
	if serverResult != true {
		t.Fatalf("server got %v, want true", serverResult)
	}
}

func TestExprDispatchUnhandledPanics(t *testing.T) {
	type bogus struct{ kont.Phantom[int] }

	ep, _ := sess.New()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for unhandled effect")
		}
		msg, ok := r.(string)
		if !ok || msg != "sess: unhandled effect in sessionHandler" {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()
	sess.ExecExpr(ep, kont.ExprPerform(bogus{}))
}

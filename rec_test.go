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

func TestLoopCounter(t *testing.T) {
	skipRace(t)
	// Counter protocol: send 0..4 via SelectR, then SelectL to close
	server := sess.Loop(0, func(acc int) kont.Eff[kont.Either[int, int]] {
		return sess.OfferBranch(
			func() kont.Eff[kont.Either[int, int]] {
				// closed
				return kont.Pure(kont.Right[int, int](acc))
			},
			func() kont.Eff[kont.Either[int, int]] {
				return sess.RecvBind(func(n int) kont.Eff[kont.Either[int, int]] {
					return kont.Pure(kont.Left[int, int](acc + n))
				})
			},
		)
	})

	client := sess.Loop(0, func(i int) kont.Eff[kont.Either[int, string]] {
		if i >= 5 {
			return sess.SelectLThen(
				sess.CloseDone(kont.Right[int, string]("done")),
			)
		}
		return sess.SelectRThen(
			sess.SendThen(i, kont.Pure(kont.Left[int, string](i+1))),
		)
	})

	clientResult, serverResult := sess.Run[string, int](client, server)
	if clientResult != "done" {
		t.Fatalf("client got %q, want %q", clientResult, "done")
	}
	// 0+1+2+3+4 = 10
	if serverResult != 10 {
		t.Fatalf("server got %d, want 10", serverResult)
	}
}

func TestLoopPingPong(t *testing.T) {
	skipRace(t)
	// Ping-pong: client sends int, server echoes doubled, accumulate until >= 100
	// Client signals close (SelectL) when done, continue (SelectR) otherwise
	server := sess.Loop(struct{}{}, func(_ struct{}) kont.Eff[kont.Either[struct{}, string]] {
		return sess.RecvBind(func(n int) kont.Eff[kont.Either[struct{}, string]] {
			return sess.SendThen(n*2,
				sess.OfferBranch(
					func() kont.Eff[kont.Either[struct{}, string]] {
						return kont.Pure(kont.Right[struct{}, string]("finished"))
					},
					func() kont.Eff[kont.Either[struct{}, string]] {
						return kont.Pure(kont.Left[struct{}, string](struct{}{}))
					},
				),
			)
		})
	})

	client := sess.Loop(1, func(n int) kont.Eff[kont.Either[int, int]] {
		return sess.SendThen(n,
			sess.RecvBind(func(doubled int) kont.Eff[kont.Either[int, int]] {
				if doubled >= 100 {
					return sess.SelectLThen(
						sess.CloseDone(kont.Right[int, int](doubled)),
					)
				}
				return sess.SelectRThen(
					kont.Pure(kont.Left[int, int](doubled)),
				)
			}),
		)
	})

	clientResult, serverResult := sess.Run[int, string](client, server)
	// 1 → 2 → 4 → 8 → 16 → 32 → 64 → 128 (≥100)
	if clientResult != 128 {
		t.Fatalf("client got %d, want 128", clientResult)
	}
	if serverResult != "finished" {
		t.Fatalf("server got %q, want %q", serverResult, "finished")
	}
}

func TestLoopImmediateTermination(t *testing.T) {
	skipRace(t)
	// Loop that terminates immediately (Right on first step)
	client := sess.Loop(0, func(_ int) kont.Eff[kont.Either[int, string]] {
		return sess.CloseDone(kont.Right[int, string]("immediate"))
	})

	server := sess.CloseDone("peer")

	clientResult, serverResult := sess.Run[string, string](client, server)
	if clientResult != "immediate" {
		t.Fatalf("client got %q, want %q", clientResult, "immediate")
	}
	if serverResult != "peer" {
		t.Fatalf("server got %q, want %q", serverResult, "peer")
	}
}

func TestLoopDeepPureStackSafe(t *testing.T) {
	skipRace(t)
	const depth = 5000000
	protocol := sess.Loop(depth, func(i int) kont.Eff[kont.Either[int, int]] {
		if i == 0 {
			return kont.Pure(kont.Right[int, int](1))
		}
		return kont.Pure(kont.Left[int, int](i - 1))
	})
	result := kont.Handle(protocol, kont.HandleFunc[int](func(op kont.Operation) (kont.Resumed, bool) {
		t.Fatalf("unexpected effect in pure loop: %T", op)
		return nil, false
	}))
	if result != 1 {
		t.Fatalf("got %d, want 1", result)
	}
}

func TestExprLoopCounter(t *testing.T) {
	skipRace(t)
	// Expr-world counter: send 0..4 via SelectR, then SelectL to close
	client := sess.ExprLoop(0, func(i int) kont.Expr[kont.Either[int, string]] {
		if i >= 5 {
			return sess.ExprSelectLThen(
				sess.ExprCloseDone(kont.Right[int, string]("done")),
			)
		}
		return sess.ExprSelectRThen(
			sess.ExprSendThen(i, kont.ExprReturn(kont.Left[int, string](i+1))),
		)
	})

	server := sess.ExprLoop(0, func(acc int) kont.Expr[kont.Either[int, int]] {
		return sess.ExprOfferBranch(
			func() kont.Expr[kont.Either[int, int]] {
				return kont.ExprReturn(kont.Right[int, int](acc))
			},
			func() kont.Expr[kont.Either[int, int]] {
				return sess.ExprRecvBind(func(n int) kont.Expr[kont.Either[int, int]] {
					return kont.ExprReturn(kont.Left[int, int](acc + n))
				})
			},
		)
	})

	clientResult, serverResult := sess.RunExpr[string, int](client, server)
	if clientResult != "done" {
		t.Fatalf("client got %q, want %q", clientResult, "done")
	}
	if serverResult != 10 {
		t.Fatalf("server got %d, want 10", serverResult)
	}
}

func TestExprLoopImmediateTermination(t *testing.T) {
	skipRace(t)
	client := sess.ExprLoop(0, func(_ int) kont.Expr[kont.Either[int, string]] {
		return sess.ExprCloseDone(kont.Right[int, string]("immediate"))
	})

	server := sess.ExprCloseDone("peer")

	clientResult, serverResult := sess.RunExpr[string, string](client, server)
	if clientResult != "immediate" {
		t.Fatalf("client got %q, want %q", clientResult, "immediate")
	}
	if serverResult != "peer" {
		t.Fatalf("server got %q, want %q", serverResult, "peer")
	}
}

func TestExprLoopPureStep(t *testing.T) {
	// Pure loop: no effects at all, only ExprReturn
	result := kont.RunPure(sess.ExprLoop(0, func(i int) kont.Expr[kont.Either[int, string]] {
		if i >= 5 {
			return kont.ExprReturn(kont.Right[int, string](fmt.Sprintf("done:%d", i)))
		}
		return kont.ExprReturn(kont.Left[int, string](i + 1))
	}))
	if result != "done:5" {
		t.Fatalf("got %q, want %q", result, "done:5")
	}
}

func TestExprLoopDeepPureStackSafe(t *testing.T) {
	const depth = 5000000
	result := kont.RunPure(sess.ExprLoop(depth, func(i int) kont.Expr[kont.Either[int, int]] {
		if i == 0 {
			return kont.ExprReturn(kont.Right[int, int](1))
		}
		return kont.ExprReturn(kont.Left[int, int](i - 1))
	}))
	if result != 1 {
		t.Fatalf("got %d, want 1", result)
	}
}

func TestLoopUltraDeepMixedEffectPureStackSafe(t *testing.T) {
	skipRace(t)
	const depth = 5000000
	const stride = 4096

	maxK := int64(depth / stride)
	expectedSum := int64(stride) * maxK * (maxK + 1) / 2

	client := sess.Loop(depth, func(i int) kont.Eff[kont.Either[int, string]] {
		if i == 0 {
			return sess.CloseDone(kont.Right[int, string]("done"))
		}
		if i%stride == 0 {
			return sess.SendThen(int64(i), kont.Pure(kont.Left[int, string](i-1)))
		}
		return kont.Pure(kont.Left[int, string](i - 1))
	})

	server := sess.Loop(struct {
		seen int
		sum  int64
	}{}, func(state struct {
		seen int
		sum  int64
	}) kont.Eff[kont.Either[struct {
		seen int
		sum  int64
	}, int64]] {
		if state.seen == int(maxK) {
			return sess.CloseDone(kont.Right[struct {
				seen int
				sum  int64
			}, int64](state.sum))
		}
		return sess.RecvBind(func(n int64) kont.Eff[kont.Either[struct {
			seen int
			sum  int64
		}, int64]] {
			return kont.Pure(kont.Left[struct {
				seen int
				sum  int64
			}, int64](struct {
				seen int
				sum  int64
			}{
				seen: state.seen + 1,
				sum:  state.sum + n,
			}))
		})
	})

	clientResult, serverResult := sess.Run[string, int64](client, server)
	if clientResult != "done" {
		t.Fatalf("client got %q, want %q", clientResult, "done")
	}
	if serverResult != expectedSum {
		t.Fatalf("sum got %d, want %d", serverResult, expectedSum)
	}
}

func TestExprLoopUltraDeepMixedEffectPureStackSafe(t *testing.T) {
	skipRace(t)
	const depth = 5000000
	const stride = 4096

	maxK := int64(depth / stride)
	expectedSum := int64(stride) * maxK * (maxK + 1) / 2

	client := sess.ExprLoop(depth, func(i int) kont.Expr[kont.Either[int, string]] {
		if i == 0 {
			return sess.ExprCloseDone(kont.Right[int, string]("done"))
		}
		if i%stride == 0 {
			return sess.ExprSendThen(int64(i), kont.ExprReturn(kont.Left[int, string](i-1)))
		}
		return kont.ExprReturn(kont.Left[int, string](i - 1))
	})

	server := sess.ExprLoop(struct {
		seen int
		sum  int64
	}{}, func(state struct {
		seen int
		sum  int64
	}) kont.Expr[kont.Either[struct {
		seen int
		sum  int64
	}, int64]] {
		if state.seen == int(maxK) {
			return sess.ExprCloseDone(kont.Right[struct {
				seen int
				sum  int64
			}, int64](state.sum))
		}
		return sess.ExprRecvBind(func(n int64) kont.Expr[kont.Either[struct {
			seen int
			sum  int64
		}, int64]] {
			return kont.ExprReturn(kont.Left[struct {
				seen int
				sum  int64
			}, int64](struct {
				seen int
				sum  int64
			}{
				seen: state.seen + 1,
				sum:  state.sum + n,
			}))
		})
	})

	clientResult, serverResult := sess.RunExpr[string, int64](client, server)
	if clientResult != "done" {
		t.Fatalf("client got %q, want %q", clientResult, "done")
	}
	if serverResult != expectedSum {
		t.Fatalf("sum got %d, want %d", serverResult, expectedSum)
	}
}

func TestExprLoopPureTermination(t *testing.T) {
	skipRace(t)
	// Mixed: effects in early iterations, pure Right on termination
	client := sess.ExprLoop(0, func(i int) kont.Expr[kont.Either[int, string]] {
		if i >= 2 {
			return kont.ExprReturn(kont.Right[int, string]("pure-done"))
		}
		return sess.ExprSendThen(i, kont.ExprReturn(kont.Left[int, string](i+1)))
	})

	server := sess.ExprRecvBind(func(a int) kont.Expr[int] {
		return sess.ExprRecvBind(func(b int) kont.Expr[int] {
			return sess.ExprCloseDone(a + b)
		})
	})

	clientResult, _ := sess.RunExpr[string, int](client, server)
	if clientResult != "pure-done" {
		t.Fatalf("client got %q, want %q", clientResult, "pure-done")
	}
}

func TestExprLoopStepping(t *testing.T) {
	skipRace(t)
	// Step through a simple loop: send 0, 1, 2 then close
	client := sess.ExprLoop(0, func(i int) kont.Expr[kont.Either[int, string]] {
		if i >= 3 {
			return sess.ExprCloseDone(kont.Right[int, string](fmt.Sprintf("sent %d", i)))
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

	epA, epB := sess.New()

	var clientResult string
	done := make(chan struct{})
	go func() {
		clientResult = execExpr(epA, client)
		close(done)
	}()
	serverResult := execExpr(epB, server)
	<-done

	if clientResult != "sent 3" {
		t.Fatalf("client got %q, want %q", clientResult, "sent 3")
	}
	// 0+1+2 = 3
	if serverResult != 3 {
		t.Fatalf("server got %d, want 3", serverResult)
	}
}

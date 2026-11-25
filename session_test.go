// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess_test

import (
	"fmt"
	"runtime"
	"slices"
	"testing"
	"weak"

	"code.hybscloud.com/iox"
	"code.hybscloud.com/kont"
	"code.hybscloud.com/sess"
)

func TestSendPayloadLifetime(t *testing.T) {
	// Use a payload larger than the runtime's tiny-object allocation batch.
	type payload [1024]byte
	for _, path := range []string{"received", "discarded"} {
		t.Run(path, func(t *testing.T) {
			sender, receiver := sess.New()
			if path == "discarded" {
				for range 4 {
					_, fill := sess.Step(kont.ExprPerform(sess.Send[*payload]{}))
					if _, next, err := sess.Advance(sender, fill); err != nil || next != nil {
						t.Fatalf("fill: next=%v, err=%v", next, err)
					}
				}
			}
			ref := func() weak.Pointer[payload] {
				value := new(payload)
				_, send := sess.Step(kont.ExprPerform(sess.Send[*payload]{Value: value}))
				_, next, err := sess.Advance(sender, send)
				if path == "discarded" {
					if !iox.IsWouldBlock(err) || next != send {
						t.Fatalf("full queue: next=%v, err=%v", next, err)
					}
					send.Discard()
				} else {
					if err != nil || next != nil {
						t.Fatalf("send: next=%v, err=%v", next, err)
					}
					_, recv := sess.Step(kont.ExprPerform(sess.Recv[*payload]{}))
					if got, next, err := sess.Advance(receiver, recv); err != nil || next != nil || got != value {
						t.Fatalf("receive: got=%p, want=%p, next=%v, err=%v", got, value, next, err)
					}
				}
				return weak.Make(value)
			}()
			runtime.GC()
			if ref.Value() != nil {
				t.Error("idle endpoints retained the payload")
			}
			runtime.KeepAlive(sender)
			runtime.KeepAlive(receiver)
		})
	}
}

func TestPairedRunnerReleasesCompletedSend(t *testing.T) {
	type payload [1024]byte
	for _, runner := range []string{"Run", "RunExpr"} {
		for _, sender := range []string{"left", "right"} {
			t.Run(runner+"/sender-"+sender, func(t *testing.T) {
				// Allocate during execution so the reusable sender holds no payload.
				left := kont.Bind(kont.Pure(struct{}{}), func(struct{}) kont.Eff[bool] {
					return sess.SendThen(new(payload), kont.Pure(true))
				})
				// Finish reading the payload before checking whether the runner retains it.
				received := kont.Map(kont.Perform(sess.Recv[*payload]{}), weak.Make[payload])
				right := kont.Map(received, func(ref weak.Pointer[payload]) bool {
					runtime.GC()
					return ref != (weak.Pointer[payload]{}) && ref.Value() == nil
				})
				if sender == "right" {
					left, right = right, left
				}

				var a, b bool
				if runner == "Run" {
					a, b = sess.Run(left, right)
				} else {
					a, b = sess.RunExpr(sess.Reify(left), sess.Reify(right))
				}
				if !a || !b {
					t.Fatal("expected the received payload to be collectible while the peer was still running")
				}
			})
		}
	}
}

func TestRunStartsBothRolesBeforeDispatch(t *testing.T) {
	var calls []string
	sender := kont.Bind(kont.Pure(41), func(n int) kont.Eff[error] {
		calls = append(calls, "start sender")
		return kont.Map(kont.Perform(sess.Send[int]{Value: n}), func(struct{}) error {
			calls = append(calls, "sent")
			return nil
		})
	})
	receiver := kont.Bind(kont.Pure(1), func(add int) kont.Eff[int] {
		calls = append(calls, "start receiver")
		return kont.Map(kont.Perform(sess.Recv[int]{}), func(n int) int {
			calls = append(calls, "received")
			return n + add
		})
	})

	for range 2 {
		calls = calls[:0]
		err, n := sess.Run(sender, receiver)
		if err != nil || n != 42 {
			t.Fatalf("Run returned (%v, %d), want (nil, 42)", err, n)
		}
		if want := []string{"start sender", "start receiver", "sent", "received"}; !slices.Equal(calls, want) {
			t.Fatalf("call order %v, want %v", calls, want)
		}
	}
}

func TestPairedRunnersPreserveImmediateResults(t *testing.T) {
	left, right := sess.Run(kont.Pure("ready"), sess.CloseDone(42))
	if left != "ready" || right != 42 {
		t.Errorf(`Run returned (%q, %d), want ("ready", 42)`, left, right)
	}
	left, right = sess.RunExpr(sess.ExprCloseDone("closed"), kont.ExprReturn(7))
	if left != "closed" || right != 7 {
		t.Errorf(`RunExpr returned (%q, %d), want ("closed", 7)`, left, right)
	}
}

func TestSendRecv(t *testing.T) {
	// !int.?string.end ↔ ?int.!string.end
	client := sess.SendThen(42,
		sess.RecvBind(func(s string) kont.Eff[string] {
			return sess.CloseDone(s)
		}),
	)

	server := sess.RecvBind(func(n int) kont.Eff[string] {
		return sess.SendThen(fmt.Sprintf("got %d", n),
			sess.CloseDone("done"),
		)
	})

	clientResult, serverResult := sess.Run[string, string](client, server)
	if clientResult != "got 42" {
		t.Fatalf("client got %q, want %q", clientResult, "got 42")
	}
	if serverResult != "done" {
		t.Fatalf("server got %q, want %q", serverResult, "done")
	}
}

func TestSendRecvMultiple(t *testing.T) {
	// !int.!int.?int.end ↔ ?int.?int.!int.end
	client := sess.SendThen(10,
		sess.SendThen(20,
			sess.RecvBind(func(sum int) kont.Eff[int] {
				return sess.CloseDone(sum)
			}),
		),
	)

	server := sess.RecvBind(func(a int) kont.Eff[int] {
		return sess.RecvBind(func(b int) kont.Eff[int] {
			return sess.SendThen(a+b, sess.CloseDone(a+b))
		})
	})

	clientResult, serverResult := sess.Run[int, int](client, server)
	if clientResult != 30 {
		t.Fatalf("client got %d, want 30", clientResult)
	}
	if serverResult != 30 {
		t.Fatalf("server got %d, want 30", serverResult)
	}
}

func TestSelectOfferLeft(t *testing.T) {
	// SelectL.!int.end ↔ Offer.?int.end
	client := sess.SelectLThen(
		sess.SendThen(99, sess.CloseDone("left")),
	)

	server := sess.OfferBranch(
		func() kont.Eff[string] {
			return sess.RecvBind(func(n int) kont.Eff[string] {
				return sess.CloseDone(fmt.Sprintf("left:%d", n))
			})
		},
		func() kont.Eff[string] {
			return sess.CloseDone("right")
		},
	)

	clientResult, serverResult := sess.Run[string, string](client, server)
	if clientResult != "left" {
		t.Fatalf("client got %q, want %q", clientResult, "left")
	}
	if serverResult != "left:99" {
		t.Fatalf("server got %q, want %q", serverResult, "left:99")
	}
}

func TestSelectOfferRight(t *testing.T) {
	// SelectR.!string.end ↔ Offer.?string.end
	client := sess.SelectRThen(
		sess.SendThen("hello", sess.CloseDone("right")),
	)

	server := sess.OfferBranch(
		func() kont.Eff[string] {
			return sess.CloseDone("left")
		},
		func() kont.Eff[string] {
			return sess.RecvBind(func(s string) kont.Eff[string] {
				return sess.CloseDone(fmt.Sprintf("right:%s", s))
			})
		},
	)

	clientResult, serverResult := sess.Run[string, string](client, server)
	if clientResult != "right" {
		t.Fatalf("client got %q, want %q", clientResult, "right")
	}
	if serverResult != "right:hello" {
		t.Fatalf("server got %q, want %q", serverResult, "right:hello")
	}
}

func TestCloseOnly(t *testing.T) {
	// end ↔ end
	a := sess.CloseDone("a")
	b := sess.CloseDone("b")

	resultA, resultB := sess.Run[string, string](a, b)
	if resultA != "a" {
		t.Fatalf("a got %q, want %q", resultA, "a")
	}
	if resultB != "b" {
		t.Fatalf("b got %q, want %q", resultB, "b")
	}
}

func TestSelectOfferReverse(t *testing.T) {
	// Server selects, client offers — exercises epB.signal and epA.await
	server := sess.SelectLThen(
		sess.SendThen(77, sess.CloseDone("selected")),
	)

	client := sess.OfferBranch(
		func() kont.Eff[string] {
			return sess.RecvBind(func(n int) kont.Eff[string] {
				return sess.CloseDone(fmt.Sprintf("got %d", n))
			})
		},
		func() kont.Eff[string] {
			return sess.CloseDone("right")
		},
	)

	clientResult, serverResult := sess.Run[string, string](client, server)
	if clientResult != "got 77" {
		t.Fatalf("client got %q, want %q", clientResult, "got 77")
	}
	if serverResult != "selected" {
		t.Fatalf("server got %q, want %q", serverResult, "selected")
	}
}

func TestDispatchUnhandledPanics(t *testing.T) {
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
	sess.Exec(ep, kont.Perform(bogus{}))
}

func TestBidirectional(t *testing.T) {
	// !int.?string.!bool.end ↔ ?int.!string.?bool.end
	client := sess.SendThen(7,
		sess.RecvBind(func(s string) kont.Eff[string] {
			return sess.SendThen(true, sess.CloseDone(s))
		}),
	)

	server := sess.RecvBind(func(n int) kont.Eff[bool] {
		return sess.SendThen(fmt.Sprintf("n=%d", n),
			sess.RecvBind(func(b bool) kont.Eff[bool] {
				return sess.CloseDone(b)
			}),
		)
	})

	clientResult, serverResult := sess.Run[string, bool](client, server)
	if clientResult != "n=7" {
		t.Fatalf("client got %q, want %q", clientResult, "n=7")
	}
	if serverResult != true {
		t.Fatalf("server got %v, want true", serverResult)
	}
}

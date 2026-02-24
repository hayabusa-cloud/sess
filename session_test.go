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

func TestSendRecv(t *testing.T) {
	skipRace(t)
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
	skipRace(t)
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
	skipRace(t)
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
	skipRace(t)
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
	skipRace(t)
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
	skipRace(t)
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
	skipRace(t)
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

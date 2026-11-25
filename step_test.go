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

func ExampleRunExpr() {
	exchange := func(n int) int {
		_, received := sess.RunExpr(
			sess.ExprSendThen(n, sess.ExprCloseDone(struct{}{})),
			sess.ExprRecvBind(sess.ExprCloseDone[int]),
		)
		return received
	}
	fmt.Println(exchange(21), exchange(42))
	// Output: 21 42
}

func ExampleAdvance() {
	sender, receiver := sess.New()
	_, h := sess.Step(kont.ExprPerform(sess.Recv[int]{}))
	_, pending, err := sess.Advance(receiver, h)
	fmt.Println(iox.IsWouldBlock(err), pending == h)

	sess.ExecExpr(sender, kont.ExprPerform(sess.Send[int]{Value: 42}))
	value, next, err := sess.Advance(receiver, pending)
	fmt.Println(value, next == nil, err)
	// Output:
	// true true
	// 42 true <nil>
}

func TestStepAdvanceSendRecv(t *testing.T) {
	skipRace(t)
	// Full protocol via Step+Advance loop
	epA, epB := sess.New()

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

	var clientResult string
	done := make(chan struct{})
	go func() {
		clientResult = execExpr(epA, client)
		close(done)
	}()
	serverResult := execExpr(epB, server)
	<-done

	if clientResult != "got 42" {
		t.Fatalf("client got %q, want %q", clientResult, "got 42")
	}
	if serverResult != "done" {
		t.Fatalf("server got %q, want %q", serverResult, "done")
	}
}

func TestStepInspectOperations(t *testing.T) {
	// susp.Op() returns concrete Send[int], Close
	protocol := sess.ExprSendThen(42, sess.ExprCloseDone[struct{}](struct{}{}))

	_, susp := sess.Step[struct{}](protocol)
	if susp == nil {
		t.Fatal("expected suspension for Send")
	}
	if _, ok := susp.Op().(sess.Send[int]); !ok {
		t.Fatalf("expected Send[int], got %T", susp.Op())
	}
	sendOp := susp.Op().(sess.Send[int])
	if sendOp.Value != 42 {
		t.Fatalf("Send value got %d, want 42", sendOp.Value)
	}

	// Dispatch the Send on an endpoint, then check next op is Close
	epA, _ := sess.New()
	_, susp, err := sess.Advance(epA, susp)
	if err != nil {
		t.Fatalf("Advance Send error: %v", err)
	}
	if susp == nil {
		t.Fatal("expected suspension for Close")
	}
	if _, ok := susp.Op().(sess.Close); !ok {
		t.Fatalf("expected Close, got %T", susp.Op())
	}

	_, susp, err = sess.Advance(epA, susp)
	if err != nil {
		t.Fatalf("Advance Close error: %v", err)
	}
	if susp != nil {
		t.Fatal("expected nil suspension after Close")
	}
}

func TestStepAdvanceSelectOffer(t *testing.T) {
	skipRace(t)
	epA, epB := sess.New()

	selector := sess.ExprSelectLThen(
		sess.ExprSendThen(77, sess.ExprCloseDone("selected")),
	)
	offerer := sess.ExprOfferBranch(
		func() kont.Expr[string] {
			return sess.ExprRecvBind(func(n int) kont.Expr[string] {
				return sess.ExprCloseDone(fmt.Sprintf("left:%d", n))
			})
		},
		func() kont.Expr[string] {
			return sess.ExprCloseDone("right")
		},
	)

	var selectResult string
	done := make(chan struct{})
	go func() {
		selectResult = execExpr(epA, selector)
		close(done)
	}()
	offerResult := execExpr(epB, offerer)
	<-done

	if selectResult != "selected" {
		t.Fatalf("selector got %q, want %q", selectResult, "selected")
	}
	if offerResult != "left:77" {
		t.Fatalf("offerer got %q, want %q", offerResult, "left:77")
	}
}

func TestStepCompletion(t *testing.T) {
	// ExprCloseDone completes with one suspension (Close), then nil
	protocol := sess.ExprCloseDone[string]("done")

	result, susp := sess.Step[string](protocol)
	if susp == nil {
		t.Fatal("expected suspension for Close")
	}
	if _, ok := susp.Op().(sess.Close); !ok {
		t.Fatalf("expected Close op, got %T", susp.Op())
	}

	ep, _ := sess.New()
	result, susp, err := sess.Advance(ep, susp)
	if err != nil {
		t.Fatalf("Advance error: %v", err)
	}
	if susp != nil {
		t.Fatal("expected nil suspension after final Close")
	}
	if result != "done" {
		t.Fatalf("result got %q, want %q", result, "done")
	}
}

func TestAdvanceWouldBlock(t *testing.T) {
	sender, receiver := sess.New()
	_, pending := sess.Step(sess.ExprRecvBind(sess.ExprCloseDone[int]))
	_, retry, err := sess.Advance(receiver, pending)
	if !iox.IsWouldBlock(err) || retry != pending {
		t.Fatalf("empty receive returned (%p, %v), want original suspension and backpressure", retry, err)
	}

	advanceOne(t, sender, kont.ExprPerform(sess.Send[int]{Value: 99}))
	_, closing, err := sess.Advance(receiver, retry)
	if err != nil || closing == nil {
		t.Fatalf("receive retry returned (%p, %v), want pending Close", closing, err)
	}
	if _, ok := closing.Op().(sess.Close); !ok {
		t.Fatalf("receive yielded %T, want Close", closing.Op())
	}
	got, next, err := sess.Advance(receiver, closing)
	if got != 99 || next != nil || err != nil {
		t.Fatalf("close returned (%d, %p, %v), want (99, nil, nil)", got, next, err)
	}
}

func TestStepAdvanceSelectOfferRight(t *testing.T) {
	skipRace(t)
	// SelectR via stepping — covers SelectR.TryDispatchSession and Offer right branch
	epA, epB := sess.New()

	selector := sess.ExprSelectRThen(
		sess.ExprSendThen("hi", sess.ExprCloseDone("right")),
	)
	offerer := sess.ExprOfferBranch(
		func() kont.Expr[string] {
			return sess.ExprCloseDone("left")
		},
		func() kont.Expr[string] {
			return sess.ExprRecvBind(func(s string) kont.Expr[string] {
				return sess.ExprCloseDone(fmt.Sprintf("right:%s", s))
			})
		},
	)

	var selectResult string
	done := make(chan struct{})
	go func() {
		selectResult = execExpr(epA, selector)
		close(done)
	}()
	offerResult := execExpr(epB, offerer)
	<-done

	if selectResult != "right" {
		t.Fatalf("selector got %q, want %q", selectResult, "right")
	}
	if offerResult != "right:hi" {
		t.Fatalf("offerer got %q, want %q", offerResult, "right:hi")
	}
}

func TestAdvanceWouldBlockSend(t *testing.T) {
	sender, receiver := sess.New()
	for n := 1; n <= 4; n++ {
		advanceOne(t, sender, kont.ExprPerform(sess.Send[int]{Value: n}))
	}
	_, pending := sess.Step(kont.ExprPerform(sess.Send[int]{Value: 5}))
	_, retry, err := sess.Advance(sender, pending)
	if !iox.IsWouldBlock(err) || retry != pending {
		t.Fatalf("full queue returned (%p, %v), want original send and backpressure", retry, err)
	}

	if got := advanceOne(t, receiver, kont.ExprPerform(sess.Recv[int]{})); got != 1 {
		t.Fatalf("first receive got %d, want 1", got)
	}
	if _, next, err := sess.Advance(sender, retry); err != nil || next != nil {
		t.Fatalf("send retry returned (%p, %v), want completion", next, err)
	}
	for want := 2; want <= 5; want++ {
		if got := advanceOne(t, receiver, kont.ExprPerform(sess.Recv[int]{})); got != want {
			t.Fatalf("received %d, want %d", got, want)
		}
	}
}

func TestAdvanceWouldBlockSelect(t *testing.T) {
	for _, tc := range []struct {
		name     string
		fifth    kont.Expr[struct{}]
		wantLeft bool
	}{
		{"left", kont.ExprPerform(sess.SelectL{}), true},
		{"right", kont.ExprPerform(sess.SelectR{}), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sender, receiver := sess.New()
			for range 4 {
				advanceOne(t, sender, kont.ExprPerform(sess.SelectL{}))
			}
			_, pending := sess.Step(tc.fifth)
			_, retry, err := sess.Advance(sender, pending)
			if !iox.IsWouldBlock(err) || retry != pending {
				t.Fatalf("full queue returned (%p, %v), want original selection and backpressure", retry, err)
			}

			// A payload receive cannot free a choice slot.
			advanceOne(t, sender, kont.ExprPerform(sess.Send[int]{Value: 42}))
			if got := advanceOne(t, receiver, kont.ExprPerform(sess.Recv[int]{})); got != 42 {
				t.Fatalf("received payload %d, want 42", got)
			}
			if _, same, err := sess.Advance(sender, retry); !iox.IsWouldBlock(err) || same != retry {
				t.Fatalf("after payload receive got (%p, %v), want selection still pending", same, err)
			}
			if offer := advanceOne(t, receiver, kont.ExprPerform(sess.Offer{})); !offer.IsLeft() {
				t.Fatal("first offer took the right branch")
			}
			if _, next, err := sess.Advance(sender, retry); err != nil || next != nil {
				t.Fatalf("selection retry returned (%p, %v), want completion", next, err)
			}
			for i := range 4 {
				gotLeft := advanceOne(t, receiver, kont.ExprPerform(sess.Offer{})).IsLeft()
				if wantLeft := i < 3 || tc.wantLeft; gotLeft != wantLeft {
					t.Fatalf("offer %d selected left=%t, want %t", i+2, gotLeft, wantLeft)
				}
			}
		})
	}
}

func TestAdvanceUnhandledPanics(t *testing.T) {
	// Advance with bogus operation panics
	type bogus struct{ kont.Phantom[int] }

	protocol := kont.ExprPerform(bogus{})

	_, susp := sess.Step[int](protocol)
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
		if !ok || msg != "sess: unhandled effect in Advance" {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()
	sess.Advance(ep, susp)
}

func TestAdvanceAffine(t *testing.T) {
	// Double susp.Resume panics
	protocol := sess.ExprCloseDone[string]("done")

	_, susp := sess.Step[string](protocol)
	if susp == nil {
		t.Fatal("expected suspension")
	}

	ep, _ := sess.New()
	_, _, err := sess.Advance(ep, susp)
	if err != nil {
		t.Fatalf("first Advance error: %v", err)
	}

	// Second Advance on same suspension should panic (affine)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on double resume")
		}
		msg, ok := r.(string)
		if !ok || msg != "kont: suspension resumed twice" {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()
	sess.Advance(ep, susp)
}

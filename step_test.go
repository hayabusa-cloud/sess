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
	skipRace(t)
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
	skipRace(t)
	// Advance returns iox.ErrWouldBlock when queue is empty, retryable
	protocol := sess.ExprRecvBind(func(n int) kont.Expr[int] {
		return sess.ExprCloseDone(n)
	})

	_, susp := sess.Step[int](protocol)
	if susp == nil {
		t.Fatal("expected suspension for Recv")
	}

	epA, epB := sess.New()

	// epA's recvQ is empty — should get ErrWouldBlock
	_, retrySusp, err := sess.Advance(epA, susp)
	if !iox.IsWouldBlock(err) {
		t.Fatalf("expected ErrWouldBlock, got %v", err)
	}
	if retrySusp != susp {
		t.Fatal("suspension should be returned unconsumed on error")
	}

	// Enqueue from peer side, then retry
	sender := sess.ExprSendThen(99, sess.ExprCloseDone[struct{}](struct{}{}))
	done := make(chan struct{})
	go func() {
		execExpr(epB, sender)
		close(done)
	}()

	// Spin-retry Advance until success
	var result int
	for {
		result, susp, err = sess.Advance(epA, susp)
		if err == nil {
			break
		}
	}
	<-done

	if result != 0 {
		// result is 0 because Recv returns value but Close follows
	}
	// Drive remaining suspensions
	for susp != nil {
		result, susp, err = sess.Advance(epA, susp)
		if err != nil {
			t.Fatalf("Advance error: %v", err)
		}
	}
	if result != 99 {
		t.Fatalf("result got %d, want 99", result)
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
	skipRace(t)
	// Advance returns iox.ErrWouldBlock when send queue is full
	protocol := sess.ExprSendThen(1,
		sess.ExprSendThen(2,
			sess.ExprSendThen(3,
				sess.ExprSendThen(4,
					sess.ExprSendThen(5, sess.ExprCloseDone[struct{}](struct{}{})),
				),
			),
		),
	)

	epA, epB := sess.New()

	// Step and advance first four sends to fill the queue (capacity=4)
	_, susp := sess.Step[struct{}](protocol)
	_, susp, err := sess.Advance(epA, susp)
	if err != nil {
		t.Fatalf("first Send: %v", err)
	}
	_, susp, err = sess.Advance(epA, susp)
	if err != nil {
		t.Fatalf("second Send: %v", err)
	}
	_, susp, err = sess.Advance(epA, susp)
	if err != nil {
		t.Fatalf("third Send: %v", err)
	}
	_, susp, err = sess.Advance(epA, susp)
	if err != nil {
		t.Fatalf("fourth Send: %v", err)
	}

	// Fifth send should get ErrWouldBlock (queue full, peer hasn't dequeued)
	_, retrySusp, err := sess.Advance(epA, susp)
	if !iox.IsWouldBlock(err) {
		t.Fatalf("expected ErrWouldBlock, got %v", err)
	}
	if retrySusp != susp {
		t.Fatal("suspension should be returned unconsumed on error")
	}

	// Drain from peer side
	receiver := sess.ExprRecvBind(func(a int) kont.Expr[int] {
		return sess.ExprRecvBind(func(b int) kont.Expr[int] {
			return sess.ExprRecvBind(func(c int) kont.Expr[int] {
				return sess.ExprRecvBind(func(d int) kont.Expr[int] {
					return sess.ExprRecvBind(func(e int) kont.Expr[int] {
						return sess.ExprCloseDone(a + b + c + d + e)
					})
				})
			})
		})
	})
	done := make(chan struct{})
	go func() {
		execExpr(epB, receiver)
		close(done)
	}()

	// Retry until success
	for susp != nil {
		_, susp, err = sess.Advance(epA, susp)
		if err != nil {
			continue
		}
	}
	<-done
}

func TestAdvanceWouldBlockSelectL(t *testing.T) {
	skipRace(t)
	// Fill signal queue, then SelectL should get ErrWouldBlock
	epA, epB := sess.New()

	// Fill signal queue: capacity=4
	protocol := sess.ExprSelectLThen(
		sess.ExprSelectLThen(
			sess.ExprSelectLThen(
				sess.ExprSelectLThen(
					sess.ExprSelectLThen(sess.ExprCloseDone[struct{}](struct{}{})),
				),
			),
		),
	)

	_, susp := sess.Step[struct{}](protocol)
	_, susp, err := sess.Advance(epA, susp)
	if err != nil {
		t.Fatalf("first SelectL: %v", err)
	}
	_, susp, err = sess.Advance(epA, susp)
	if err != nil {
		t.Fatalf("second SelectL: %v", err)
	}
	_, susp, err = sess.Advance(epA, susp)
	if err != nil {
		t.Fatalf("third SelectL: %v", err)
	}
	_, susp, err = sess.Advance(epA, susp)
	if err != nil {
		t.Fatalf("fourth SelectL: %v", err)
	}

	// Fifth select should block
	_, retrySusp, err := sess.Advance(epA, susp)
	if !iox.IsWouldBlock(err) {
		t.Fatalf("expected ErrWouldBlock, got %v", err)
	}
	if retrySusp != susp {
		t.Fatal("suspension should be returned unconsumed on error")
	}

	// Drain from peer side
	offerer := sess.ExprOfferBranch(
		func() kont.Expr[struct{}] {
			return sess.ExprOfferBranch(
				func() kont.Expr[struct{}] {
					return sess.ExprOfferBranch(
						func() kont.Expr[struct{}] {
							return sess.ExprOfferBranch(
								func() kont.Expr[struct{}] {
									return sess.ExprOfferBranch(
										func() kont.Expr[struct{}] { return sess.ExprCloseDone[struct{}](struct{}{}) },
										func() kont.Expr[struct{}] { return sess.ExprCloseDone[struct{}](struct{}{}) },
									)
								},
								func() kont.Expr[struct{}] { return sess.ExprCloseDone[struct{}](struct{}{}) },
							)
						},
						func() kont.Expr[struct{}] { return sess.ExprCloseDone[struct{}](struct{}{}) },
					)
				},
				func() kont.Expr[struct{}] { return sess.ExprCloseDone[struct{}](struct{}{}) },
			)
		},
		func() kont.Expr[struct{}] { return sess.ExprCloseDone[struct{}](struct{}{}) },
	)
	done := make(chan struct{})
	go func() {
		execExpr(epB, offerer)
		close(done)
	}()

	for susp != nil {
		_, susp, err = sess.Advance(epA, susp)
		if err != nil {
			continue
		}
	}
	<-done
}

func TestAdvanceWouldBlockSelectR(t *testing.T) {
	skipRace(t)
	// Fill signal queue with SelectR, then should get ErrWouldBlock
	epA, epB := sess.New()

	protocol := sess.ExprSelectRThen(
		sess.ExprSelectRThen(
			sess.ExprSelectRThen(
				sess.ExprSelectRThen(
					sess.ExprSelectRThen(sess.ExprCloseDone[struct{}](struct{}{})),
				),
			),
		),
	)

	_, susp := sess.Step[struct{}](protocol)
	_, susp, err := sess.Advance(epA, susp)
	if err != nil {
		t.Fatalf("first SelectR: %v", err)
	}
	_, susp, err = sess.Advance(epA, susp)
	if err != nil {
		t.Fatalf("second SelectR: %v", err)
	}
	_, susp, err = sess.Advance(epA, susp)
	if err != nil {
		t.Fatalf("third SelectR: %v", err)
	}
	_, susp, err = sess.Advance(epA, susp)
	if err != nil {
		t.Fatalf("fourth SelectR: %v", err)
	}

	// Fifth select should block
	_, retrySusp, err := sess.Advance(epA, susp)
	if !iox.IsWouldBlock(err) {
		t.Fatalf("expected ErrWouldBlock, got %v", err)
	}
	if retrySusp != susp {
		t.Fatal("suspension should be returned unconsumed on error")
	}

	// Drain from peer side
	offerer := sess.ExprOfferBranch(
		func() kont.Expr[struct{}] { return sess.ExprCloseDone[struct{}](struct{}{}) },
		func() kont.Expr[struct{}] {
			return sess.ExprOfferBranch(
				func() kont.Expr[struct{}] { return sess.ExprCloseDone[struct{}](struct{}{}) },
				func() kont.Expr[struct{}] {
					return sess.ExprOfferBranch(
						func() kont.Expr[struct{}] { return sess.ExprCloseDone[struct{}](struct{}{}) },
						func() kont.Expr[struct{}] {
							return sess.ExprOfferBranch(
								func() kont.Expr[struct{}] { return sess.ExprCloseDone[struct{}](struct{}{}) },
								func() kont.Expr[struct{}] {
									return sess.ExprOfferBranch(
										func() kont.Expr[struct{}] { return sess.ExprCloseDone[struct{}](struct{}{}) },
										func() kont.Expr[struct{}] { return sess.ExprCloseDone[struct{}](struct{}{}) },
									)
								},
							)
						},
					)
				},
			)
		},
	)
	done := make(chan struct{})
	go func() {
		execExpr(epB, offerer)
		close(done)
	}()

	for susp != nil {
		_, susp, err = sess.Advance(epA, susp)
		if err != nil {
			continue
		}
	}
	<-done
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

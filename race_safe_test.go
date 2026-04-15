// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess_test

import (
	"testing"

	"code.hybscloud.com/kont"
	"code.hybscloud.com/sess"
)

func TestRunCloseOnlyRaceSafe(t *testing.T) {
	left, right := sess.Run(
		sess.CloseDone("left"),
		sess.CloseDone("right"),
	)
	if left != "left" {
		t.Fatalf("left got %q, want %q", left, "left")
	}
	if right != "right" {
		t.Fatalf("right got %q, want %q", right, "right")
	}
}

func TestRunErrorCloseOnlyRaceSafe(t *testing.T) {
	left, right, thrown := sess.RunError[string](
		sess.CloseDone("left"),
		sess.CloseDone("right"),
	)
	if thrown != nil {
		t.Fatalf("expected nil thrown, got %v", *thrown)
	}
	if !left.IsRight() {
		t.Fatal("left expected Right, got Left")
	}
	if !right.IsRight() {
		t.Fatal("right expected Right, got Left")
	}
	leftValue, _ := left.GetRight()
	rightValue, _ := right.GetRight()
	if leftValue != "left" {
		t.Fatalf("left got %q, want %q", leftValue, "left")
	}
	if rightValue != "right" {
		t.Fatalf("right got %q, want %q", rightValue, "right")
	}
}

func TestExecReflectCloseOnlyRaceSafe(t *testing.T) {
	ep, _ := sess.New()
	result := sess.Exec(ep, sess.Reflect(sess.ExprCloseDone("ok")))
	if result != "ok" {
		t.Fatalf("got %q, want %q", result, "ok")
	}
}

func TestExecErrorExprCloseOnlyRaceSafe(t *testing.T) {
	ep, _ := sess.New()
	result := sess.ExecErrorExpr[string](ep, sess.ExprCloseDone("ok"))
	if !result.IsRight() {
		t.Fatal("expected Right, got Left")
	}
	value, _ := result.GetRight()
	if value != "ok" {
		t.Fatalf("got %q, want %q", value, "ok")
	}
}

func TestExecErrorPureThrowRaceSafe(t *testing.T) {
	ep, _ := sess.New()
	result := sess.ExecError[string](ep, kont.ThrowError[string, int]("boom"))
	if !result.IsLeft() {
		t.Fatal("expected Left, got Right")
	}
	value, _ := result.GetLeft()
	if value != "boom" {
		t.Fatalf("got %q, want %q", value, "boom")
	}
}

func TestContStepOpsRaceSafe(t *testing.T) {
	t.Run("send", func(t *testing.T) {
		ep, _ := sess.New()
		_, susp := sess.Step(sess.Reify(sess.SendThen(1, sess.CloseDone("done"))))
		if susp == nil {
			t.Fatal("expected suspension")
		}
		if _, ok := susp.Op().(sess.Send[int]); !ok {
			t.Fatalf("expected Send[int], got %T", susp.Op())
		}
		_, next, err := sess.Advance(ep, susp)
		if err != nil {
			t.Fatalf("advance send: %v", err)
		}
		if next == nil {
			t.Fatal("expected close suspension")
		}
		if _, ok := next.Op().(sess.Close); !ok {
			t.Fatalf("expected Close, got %T", next.Op())
		}
	})

	t.Run("recv", func(t *testing.T) {
		epA, epB := sess.New()
		_, recvSusp := sess.Step(sess.Reify(sess.RecvBind(func(n int) kont.Eff[int] {
			return sess.CloseDone(n + 1)
		})))
		if recvSusp == nil {
			t.Fatal("expected recv suspension")
		}
		_, sendSusp := sess.Step(sess.ExprSendThen(41, sess.ExprCloseDone(struct{}{})))
		if sendSusp == nil {
			t.Fatal("expected send suspension")
		}
		if _, _, err := sess.Advance(epB, sendSusp); err != nil {
			t.Fatalf("advance peer send: %v", err)
		}
		result, next, err := sess.Advance(epA, recvSusp)
		if err != nil {
			t.Fatalf("advance recv: %v", err)
		}
		if result != 0 {
			t.Fatalf("recv result got %d, want 0 before close", result)
		}
		if next == nil {
			t.Fatal("expected close suspension")
		}
		if _, ok := next.Op().(sess.Close); !ok {
			t.Fatalf("expected Close, got %T", next.Op())
		}
		result, next, err = sess.Advance(epA, next)
		if err != nil {
			t.Fatalf("advance close: %v", err)
		}
		if next != nil {
			t.Fatal("expected completion")
		}
		if result != 42 {
			t.Fatalf("got %d, want 42", result)
		}
	})

	t.Run("select-left", func(t *testing.T) {
		ep, _ := sess.New()
		_, susp := sess.Step(sess.Reify(sess.SelectLThen(sess.CloseDone("left"))))
		if susp == nil {
			t.Fatal("expected select suspension")
		}
		if _, ok := susp.Op().(sess.SelectL); !ok {
			t.Fatalf("expected SelectL, got %T", susp.Op())
		}
		_, next, err := sess.Advance(ep, susp)
		if err != nil {
			t.Fatalf("advance select-left: %v", err)
		}
		if next == nil {
			t.Fatal("expected close suspension")
		}
		if _, ok := next.Op().(sess.Close); !ok {
			t.Fatalf("expected Close, got %T", next.Op())
		}
	})

	t.Run("select-right", func(t *testing.T) {
		ep, _ := sess.New()
		_, susp := sess.Step(sess.Reify(sess.SelectRThen(sess.CloseDone("right"))))
		if susp == nil {
			t.Fatal("expected select suspension")
		}
		if _, ok := susp.Op().(sess.SelectR); !ok {
			t.Fatalf("expected SelectR, got %T", susp.Op())
		}
		_, next, err := sess.Advance(ep, susp)
		if err != nil {
			t.Fatalf("advance select-right: %v", err)
		}
		if next == nil {
			t.Fatal("expected close suspension")
		}
		if _, ok := next.Op().(sess.Close); !ok {
			t.Fatalf("expected Close, got %T", next.Op())
		}
	})

	t.Run("offer-left", func(t *testing.T) {
		epA, epB := sess.New()
		_, offerSusp := sess.Step(sess.Reify(sess.OfferBranch(
			func() kont.Eff[string] { return sess.CloseDone("left") },
			func() kont.Eff[string] { return sess.CloseDone("right") },
		)))
		if offerSusp == nil {
			t.Fatal("expected offer suspension")
		}
		_, selectSusp := sess.Step(sess.ExprSelectLThen(sess.ExprCloseDone(struct{}{})))
		if selectSusp == nil {
			t.Fatal("expected select suspension")
		}
		if _, _, err := sess.Advance(epB, selectSusp); err != nil {
			t.Fatalf("advance peer select: %v", err)
		}
		result, next, err := sess.Advance(epA, offerSusp)
		if err != nil {
			t.Fatalf("advance offer: %v", err)
		}
		if result != "" {
			t.Fatalf("offer result got %q, want empty before close", result)
		}
		if next == nil {
			t.Fatal("expected close suspension")
		}
		if _, ok := next.Op().(sess.Close); !ok {
			t.Fatalf("expected Close, got %T", next.Op())
		}
		result, next, err = sess.Advance(epA, next)
		if err != nil {
			t.Fatalf("advance close: %v", err)
		}
		if next != nil {
			t.Fatal("expected completion")
		}
		if result != "left" {
			t.Fatalf("got %q, want %q", result, "left")
		}
	})

	t.Run("offer-right", func(t *testing.T) {
		epA, epB := sess.New()
		_, offerSusp := sess.Step(sess.Reify(sess.OfferBranch(
			func() kont.Eff[string] { return sess.CloseDone("left") },
			func() kont.Eff[string] { return sess.CloseDone("right") },
		)))
		if offerSusp == nil {
			t.Fatal("expected offer suspension")
		}
		_, selectSusp := sess.Step(sess.ExprSelectRThen(sess.ExprCloseDone(struct{}{})))
		if selectSusp == nil {
			t.Fatal("expected select suspension")
		}
		if _, _, err := sess.Advance(epB, selectSusp); err != nil {
			t.Fatalf("advance peer select: %v", err)
		}
		result, next, err := sess.Advance(epA, offerSusp)
		if err != nil {
			t.Fatalf("advance offer: %v", err)
		}
		if result != "" {
			t.Fatalf("offer result got %q, want empty before close", result)
		}
		if next == nil {
			t.Fatal("expected close suspension")
		}
		if _, ok := next.Op().(sess.Close); !ok {
			t.Fatalf("expected Close, got %T", next.Op())
		}
		result, next, err = sess.Advance(epA, next)
		if err != nil {
			t.Fatalf("advance close: %v", err)
		}
		if next != nil {
			t.Fatal("expected completion")
		}
		if result != "right" {
			t.Fatalf("got %q, want %q", result, "right")
		}
	})
}

func TestExprStepAndResumeRaceSafe(t *testing.T) {
	t.Run("send", func(t *testing.T) {
		_, susp := sess.Step(sess.ExprSendThen(1, sess.ExprCloseDone("done")))
		if susp == nil {
			t.Fatal("expected suspension")
		}
		if _, ok := susp.Op().(sess.Send[int]); !ok {
			t.Fatalf("expected Send[int], got %T", susp.Op())
		}
	})

	t.Run("recv", func(t *testing.T) {
		_, susp := sess.Step(sess.ExprRecvBind(func(n int) kont.Expr[int] {
			return sess.ExprCloseDone(n + 1)
		}))
		if susp == nil {
			t.Fatal("expected recv suspension")
		}
		result, next := susp.Resume(41)
		if result != 0 {
			t.Fatalf("recv result got %d, want 0 before close", result)
		}
		if next == nil {
			t.Fatal("expected close suspension")
		}
		if _, ok := next.Op().(sess.Close); !ok {
			t.Fatalf("expected Close, got %T", next.Op())
		}
	})

	t.Run("select-left", func(t *testing.T) {
		_, susp := sess.Step(sess.ExprSelectLThen(sess.ExprCloseDone("left")))
		if susp == nil {
			t.Fatal("expected select suspension")
		}
		if _, ok := susp.Op().(sess.SelectL); !ok {
			t.Fatalf("expected SelectL, got %T", susp.Op())
		}
	})

	t.Run("select-right", func(t *testing.T) {
		_, susp := sess.Step(sess.ExprSelectRThen(sess.ExprCloseDone("right")))
		if susp == nil {
			t.Fatal("expected select suspension")
		}
		if _, ok := susp.Op().(sess.SelectR); !ok {
			t.Fatalf("expected SelectR, got %T", susp.Op())
		}
	})

	t.Run("offer-left", func(t *testing.T) {
		_, susp := sess.Step(sess.ExprOfferBranch(
			func() kont.Expr[string] { return sess.ExprCloseDone("left") },
			func() kont.Expr[string] { return sess.ExprCloseDone("right") },
		))
		if susp == nil {
			t.Fatal("expected offer suspension")
		}
		result, next := susp.Resume(kont.Left[struct{}, struct{}](struct{}{}))
		if result != "" {
			t.Fatalf("offer result got %q, want empty before close", result)
		}
		if next == nil {
			t.Fatal("expected close suspension")
		}
		if _, ok := next.Op().(sess.Close); !ok {
			t.Fatalf("expected Close, got %T", next.Op())
		}
	})

	t.Run("offer-right", func(t *testing.T) {
		_, susp := sess.Step(sess.ExprOfferBranch(
			func() kont.Expr[string] { return sess.ExprCloseDone("left") },
			func() kont.Expr[string] { return sess.ExprCloseDone("right") },
		))
		if susp == nil {
			t.Fatal("expected offer suspension")
		}
		result, next := susp.Resume(kont.Right[struct{}](struct{}{}))
		if result != "" {
			t.Fatalf("offer result got %q, want empty before close", result)
		}
		if next == nil {
			t.Fatal("expected close suspension")
		}
		if _, ok := next.Op().(sess.Close); !ok {
			t.Fatalf("expected Close, got %T", next.Op())
		}
	})
}

func TestAdvanceWouldBlockRaceSafe(t *testing.T) {
	t.Run("send-full", func(t *testing.T) {
		ep, _ := sess.New()
		protocol := sess.ExprSendThen(1,
			sess.ExprSendThen(2,
				sess.ExprSendThen(3,
					sess.ExprSendThen(4,
						sess.ExprSendThen(5, sess.ExprCloseDone(struct{}{})),
					),
				),
			),
		)
		_, susp := sess.Step(protocol)
		for i := 0; i < 4; i++ {
			if _, next, err := sess.Advance(ep, susp); err != nil {
				t.Fatalf("advance send %d: %v", i+1, err)
			} else {
				susp = next
			}
		}
		if _, retry, err := sess.Advance(ep, susp); err == nil {
			t.Fatal("expected ErrWouldBlock on full send queue")
		} else if retry != susp {
			t.Fatal("expected same suspension on WouldBlock")
		}
	})

	t.Run("select-left-full", func(t *testing.T) {
		ep, _ := sess.New()
		protocol := sess.ExprSelectLThen(
			sess.ExprSelectLThen(
				sess.ExprSelectLThen(
					sess.ExprSelectLThen(
						sess.ExprSelectLThen(sess.ExprCloseDone(struct{}{})),
					),
				),
			),
		)
		_, susp := sess.Step(protocol)
		for i := 0; i < 4; i++ {
			if _, next, err := sess.Advance(ep, susp); err != nil {
				t.Fatalf("advance select-left %d: %v", i+1, err)
			} else {
				susp = next
			}
		}
		if _, retry, err := sess.Advance(ep, susp); err == nil {
			t.Fatal("expected ErrWouldBlock on full select-left queue")
		} else if retry != susp {
			t.Fatal("expected same suspension on WouldBlock")
		}
	})

	t.Run("select-right-full", func(t *testing.T) {
		ep, _ := sess.New()
		protocol := sess.ExprSelectRThen(
			sess.ExprSelectRThen(
				sess.ExprSelectRThen(
					sess.ExprSelectRThen(
						sess.ExprSelectRThen(sess.ExprCloseDone(struct{}{})),
					),
				),
			),
		)
		_, susp := sess.Step(protocol)
		for i := 0; i < 4; i++ {
			if _, next, err := sess.Advance(ep, susp); err != nil {
				t.Fatalf("advance select-right %d: %v", i+1, err)
			} else {
				susp = next
			}
		}
		if _, retry, err := sess.Advance(ep, susp); err == nil {
			t.Fatal("expected ErrWouldBlock on full select-right queue")
		} else if retry != susp {
			t.Fatal("expected same suspension on WouldBlock")
		}
	})

	t.Run("offer-empty", func(t *testing.T) {
		ep, _ := sess.New()
		_, susp := sess.Step(sess.Reify(sess.OfferBranch(
			func() kont.Eff[string] { return sess.CloseDone("left") },
			func() kont.Eff[string] { return sess.CloseDone("right") },
		)))
		if susp == nil {
			t.Fatal("expected offer suspension")
		}
		if _, retry, err := sess.Advance(ep, susp); err == nil {
			t.Fatal("expected ErrWouldBlock on empty offer queue")
		} else if retry != susp {
			t.Fatal("expected same suspension on WouldBlock")
		}
	})
}

func TestLoopEffectfulCloseOnlyRaceSafe(t *testing.T) {
	ep, _ := sess.New()
	result := sess.Exec(ep, sess.Loop(0, func(i int) kont.Eff[kont.Either[int, int]] {
		if i == 0 {
			return sess.CloseDone(kont.Left[int, int](1))
		}
		return kont.Pure(kont.Right[int, int](i))
	}))
	if result != 1 {
		t.Fatalf("got %d, want 1", result)
	}
}

func TestExprLoopEffectfulCloseOnlyRaceSafe(t *testing.T) {
	ep, _ := sess.New()
	result, susp := sess.Step(sess.ExprLoop(0, func(i int) kont.Expr[kont.Either[int, int]] {
		if i == 0 {
			return sess.ExprCloseDone(kont.Left[int, int](1))
		}
		return kont.ExprReturn(kont.Right[int, int](i))
	}))
	if result != 0 {
		t.Fatalf("got %d, want 0 before close", result)
	}
	if susp == nil {
		t.Fatal("expected close suspension")
	}
	if _, ok := susp.Op().(sess.Close); !ok {
		t.Fatalf("expected Close, got %T", susp.Op())
	}
	result, susp, err := sess.Advance(ep, susp)
	if err != nil {
		t.Fatalf("advance close: %v", err)
	}
	if susp != nil {
		t.Fatal("expected completion")
	}
	if result != 1 {
		t.Fatalf("got %d, want 1", result)
	}
}

func TestExprLoopEffectfulRightCloseOnlyRaceSafe(t *testing.T) {
	ep, _ := sess.New()
	result, susp := sess.Step(sess.ExprLoop(0, func(int) kont.Expr[kont.Either[int, int]] {
		return sess.ExprCloseDone(kont.Right[int, int](7))
	}))
	if result != 0 {
		t.Fatalf("got %d, want 0 before close", result)
	}
	if susp == nil {
		t.Fatal("expected close suspension")
	}
	result, susp, err := sess.Advance(ep, susp)
	if err != nil {
		t.Fatalf("advance close: %v", err)
	}
	if susp != nil {
		t.Fatal("expected completion")
	}
	if result != 7 {
		t.Fatalf("got %d, want 7", result)
	}
}

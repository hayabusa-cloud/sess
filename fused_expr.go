// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess

import (
	"code.hybscloud.com/kont"
)

// identityResume is the identity resume function for EffectFrame construction.
// Named function produces a static function value, consistent with kont convention.
func identityResume(v kont.Erased) kont.Erased { return v }

// ExprSendThen sends a value and then continues with next.
// Fuses ExprPerform(Send[T]{Value: v}) + ExprThen.
func ExprSendThen[T, B any](v T, next kont.Expr[B]) kont.Expr[B] {
	tf := kont.AcquireThenFrame()
	tf.Second = kont.Expr[kont.Erased]{Value: kont.Erased(next.Value), Frame: next.Frame}
	tf.Next = kont.ReturnFrame{}
	ef := kont.AcquireEffectFrame()
	ef.Operation = Send[T]{Value: v}
	ef.Resume = identityResume
	ef.Next = tf
	return kont.ExprSuspend[B](ef)
}

// ExprRecvBind receives a value and passes it to f.
// Fuses ExprPerform(Recv[T]{}) + ExprBind.
func ExprRecvBind[T, B any](f func(T) kont.Expr[B]) kont.Expr[B] {
	bf := kont.AcquireBindFrame()
	bf.F = func(a kont.Erased) kont.Expr[kont.Erased] {
		result := f(a.(T))
		return kont.Expr[kont.Erased]{Value: kont.Erased(result.Value), Frame: result.Frame}
	}
	bf.Next = kont.ReturnFrame{}
	ef := kont.AcquireEffectFrame()
	ef.Operation = Recv[T]{}
	ef.Resume = identityResume
	ef.Next = bf
	return kont.ExprSuspend[B](ef)
}

// ExprCloseDone closes the session and returns a.
// Fuses ExprPerform(Close{}) + ExprReturn.
func ExprCloseDone[A any](a A) kont.Expr[A] {
	tf := kont.AcquireThenFrame()
	tf.Second = kont.Expr[kont.Erased]{Value: kont.Erased(a), Frame: kont.ReturnFrame{}}
	tf.Next = kont.ReturnFrame{}
	ef := kont.AcquireEffectFrame()
	ef.Operation = Close{}
	ef.Resume = identityResume
	ef.Next = tf
	return kont.ExprSuspend[A](ef)
}

// ExprSelectLThen selects the left branch and continues with next.
// Fuses ExprPerform(SelectL{}) + ExprThen.
func ExprSelectLThen[B any](next kont.Expr[B]) kont.Expr[B] {
	tf := kont.AcquireThenFrame()
	tf.Second = kont.Expr[kont.Erased]{Value: kont.Erased(next.Value), Frame: next.Frame}
	tf.Next = kont.ReturnFrame{}
	ef := kont.AcquireEffectFrame()
	ef.Operation = SelectL{}
	ef.Resume = identityResume
	ef.Next = tf
	return kont.ExprSuspend[B](ef)
}

// ExprSelectRThen selects the right branch and continues with next.
// Fuses ExprPerform(SelectR{}) + ExprThen.
func ExprSelectRThen[B any](next kont.Expr[B]) kont.Expr[B] {
	tf := kont.AcquireThenFrame()
	tf.Second = kont.Expr[kont.Erased]{Value: kont.Erased(next.Value), Frame: next.Frame}
	tf.Next = kont.ReturnFrame{}
	ef := kont.AcquireEffectFrame()
	ef.Operation = SelectR{}
	ef.Resume = identityResume
	ef.Next = tf
	return kont.ExprSuspend[B](ef)
}

// ExprOfferBranch waits for the peer's choice and calls onLeft or onRight.
// Fuses ExprPerform(Offer{}) + ExprBind + Either branch.
func ExprOfferBranch[A any](onLeft func() kont.Expr[A], onRight func() kont.Expr[A]) kont.Expr[A] {
	bf := kont.AcquireBindFrame()
	bf.F = func(a kont.Erased) kont.Expr[kont.Erased] {
		e := a.(kont.Either[struct{}, struct{}])
		var result kont.Expr[A]
		if e.IsLeft() {
			result = onLeft()
		} else {
			result = onRight()
		}
		return kont.Expr[kont.Erased]{Value: kont.Erased(result.Value), Frame: result.Frame}
	}
	bf.Next = kont.ReturnFrame{}
	ef := kont.AcquireEffectFrame()
	ef.Operation = Offer{}
	ef.Resume = identityResume
	ef.Next = bf
	return kont.ExprSuspend[A](ef)
}

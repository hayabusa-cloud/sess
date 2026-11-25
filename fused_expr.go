// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess

import (
	"code.hybscloud.com/kont"
)

// Pre-allocated erased operations and frames to eliminate heap escapes
// when boxing empty structs into any/kont.Frame during Expr-world execution.
var (
	exprReturnFrame kont.Frame  = kont.ReturnFrame{}
	exprClose       kont.Erased = Close{}
	exprSelectL     kont.Erased = SelectL{}
	exprSelectR     kont.Erased = SelectR{}
	exprOffer       kont.Erased = Offer{}
)

// identityResume is the identity resume function for EffectFrame construction.
// Named function produces a static function value, consistent with kont convention.
func identityResume(v kont.Erased) kont.Erased { return v }

// effectThen sequences op before next, discarding op's reply.
func effectThen[A any](op kont.Operation, next *kont.Expr[A]) *kont.EffectFrame[kont.Erased] {
	ef := kont.AcquireEffectFrame()
	ef.Operation = op
	ef.Resume = identityResume
	if _, ok := next.Frame.(*kont.EffectFrame[kont.Erased]); ok {
		// A following effect reads its own dispatch reply.
		ef.Next = next.Frame
		return ef
	}
	tf := kont.AcquireThenFrame()
	tf.Second = kont.Expr[kont.Erased]{Value: next.Value, Frame: next.Frame}
	tf.Next = exprReturnFrame
	ef.Next = tf
	return ef
}

// ExprSendThen builds a single-use expression that sends v and continues with next.
// Fuses ExprPerform(Send[T]{Value: v}) + ExprThen.
func ExprSendThen[T, B any](v T, next kont.Expr[B]) kont.Expr[B] {
	return kont.ExprSuspend[B](effectThen(Send[T]{Value: v}, &next))
}

func recvBindUnwind[T, B any](data, _, _ kont.Erased, current kont.Erased) (kont.Erased, kont.Frame) {
	f := data.(func(T) kont.Expr[B])
	result := f(current.(T))
	return kont.Erased(result.Value), result.Frame
}

// ExprRecvBind builds a single-use expression that receives a value and passes it to f.
// Fuses ExprPerform(Recv[T]{}) + ExprBind.
func ExprRecvBind[T, B any](f func(T) kont.Expr[B]) kont.Expr[B] {
	bf := kont.AcquireUnwindFrame()
	bf.Data1 = f
	bf.Unwind = recvBindUnwind[T, B]
	ef := kont.AcquireEffectFrame()
	ef.Operation = Recv[T]{}
	ef.Resume = identityResume
	ef.Next = bf
	return kont.ExprSuspend[B](ef)
}

// ExprCloseDone builds a single-use expression that closes the session and returns a.
// Fuses ExprPerform(Close{}) + ExprThen + ExprReturn.
func ExprCloseDone[A any](a A) kont.Expr[A] {
	tf := kont.AcquireThenFrame()
	tf.Second = kont.Expr[kont.Erased]{Value: kont.Erased(a), Frame: exprReturnFrame}
	tf.Next = exprReturnFrame
	ef := kont.AcquireEffectFrame()
	ef.Operation = exprClose
	ef.Resume = identityResume
	ef.Next = tf
	return kont.ExprSuspend[A](ef)
}

// ExprSelectLThen builds a single-use expression that selects the left branch and continues with next.
// Fuses ExprPerform(SelectL{}) + ExprThen.
func ExprSelectLThen[B any](next kont.Expr[B]) kont.Expr[B] {
	return kont.ExprSuspend[B](effectThen(exprSelectL, &next))
}

// ExprSelectRThen builds a single-use expression that selects the right branch and continues with next.
// Fuses ExprPerform(SelectR{}) + ExprThen.
func ExprSelectRThen[B any](next kont.Expr[B]) kont.Expr[B] {
	return kont.ExprSuspend[B](effectThen(exprSelectR, &next))
}

func offerBranchUnwind[A any](data, data2, _ kont.Erased, current kont.Erased) (kont.Erased, kont.Frame) {
	onLeft := data.(func() kont.Expr[A])
	onRight := data2.(func() kont.Expr[A])
	e := current.(kont.Either[struct{}, struct{}])
	var result kont.Expr[A]
	if e.IsLeft() {
		result = onLeft()
	} else {
		result = onRight()
	}
	return kont.Erased(result.Value), result.Frame
}

// ExprOfferBranch builds a single-use expression that calls onLeft or onRight on the peer's choice.
// Fuses ExprPerform(Offer{}) + ExprBind + Either branch.
func ExprOfferBranch[A any](onLeft func() kont.Expr[A], onRight func() kont.Expr[A]) kont.Expr[A] {
	bf := kont.AcquireUnwindFrame()
	bf.Data1 = onLeft
	bf.Data2 = onRight
	bf.Unwind = offerBranchUnwind[A]
	ef := kont.AcquireEffectFrame()
	ef.Operation = exprOffer
	ef.Resume = identityResume
	ef.Next = bf
	return kont.ExprSuspend[A](ef)
}

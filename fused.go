// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess

import (
	"code.hybscloud.com/kont"
)

// SendThen sends a value and then continues with next.
// Fuses Perform(Send[T]{Value: v}) + Then.
func SendThen[T, B any](v T, next kont.Eff[B]) kont.Eff[B] {
	return kont.Then(kont.Perform(Send[T]{Value: v}), next)
}

// RecvBind receives a value and passes it to f.
// Fuses Perform(Recv[T]{}) + Bind.
func RecvBind[T, B any](f func(T) kont.Eff[B]) kont.Eff[B] {
	return kont.Bind(kont.Perform(Recv[T]{}), f)
}

// CloseDone closes the session and returns a.
// Fuses Perform(Close{}) + Then + Pure.
func CloseDone[A any](a A) kont.Eff[A] {
	return kont.Then(kont.Perform(Close{}), kont.Pure(a))
}

// SelectLThen selects the left branch and continues with next.
// Fuses Perform(SelectL{}) + Then.
func SelectLThen[B any](next kont.Eff[B]) kont.Eff[B] {
	return kont.Then(kont.Perform(SelectL{}), next)
}

// SelectRThen selects the right branch and continues with next.
// Fuses Perform(SelectR{}) + Then.
func SelectRThen[B any](next kont.Eff[B]) kont.Eff[B] {
	return kont.Then(kont.Perform(SelectR{}), next)
}

// OfferBranch waits for the peer's choice and calls onLeft or onRight.
// Fuses Perform(Offer{}) + Bind + Either branch.
func OfferBranch[A any](onLeft func() kont.Eff[A], onRight func() kont.Eff[A]) kont.Eff[A] {
	return kont.Bind(kont.Perform(Offer{}), func(e kont.Either[struct{}, struct{}]) kont.Eff[A] {
		if e.IsLeft() {
			return onLeft()
		}
		return onRight()
	})
}

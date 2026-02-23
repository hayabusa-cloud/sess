// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess

import (
	"code.hybscloud.com/kont"
)

// Loop runs a recursive session protocol (Cont-world).
// step returns Left(nextState) to continue or Right(result) to finish.
func Loop[S, A any](initial S, step func(S) kont.Eff[kont.Either[S, A]]) kont.Eff[A] {
	return kont.Bind(step(initial), func(e kont.Either[S, A]) kont.Eff[A] {
		if left, ok := e.GetLeft(); ok {
			return Loop(left, step)
		}
		right, _ := e.GetRight()
		return kont.Pure(right)
	})
}

// ExprLoop runs a recursive session protocol (Expr-world).
// step returns Left(nextState) to continue or Right(result) to finish.
// Fuses ExprBind inline to avoid the type-erasing wrapper closure.
func ExprLoop[S, A any](initial S, step func(S) kont.Expr[kont.Either[S, A]]) kont.Expr[A] {
	m := step(initial)
	if _, ok := m.Frame.(kont.ReturnFrame); ok {
		if left, ok := m.Value.GetLeft(); ok {
			return ExprLoop(left, step)
		}
		right, _ := m.Value.GetRight()
		return kont.ExprReturn(right)
	}
	bf := kont.AcquireBindFrame()
	bf.F = func(a kont.Erased) kont.Expr[kont.Erased] {
		e := a.(kont.Either[S, A])
		if left, ok := e.GetLeft(); ok {
			result := ExprLoop(left, step)
			return kont.Expr[kont.Erased]{Value: kont.Erased(result.Value), Frame: result.Frame}
		}
		right, _ := e.GetRight()
		return kont.Expr[kont.Erased]{Value: kont.Erased(right), Frame: kont.ReturnFrame{}}
	}
	bf.Next = kont.ReturnFrame{}
	var zero A
	return kont.Expr[A]{
		Value: zero,
		Frame: kont.ChainFrames(m.Frame, bf),
	}
}

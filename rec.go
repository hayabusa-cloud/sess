// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess

import (
	"code.hybscloud.com/kont"
)

// Loop runs a recursive session protocol (Cont-world).
// step returns Left(nextState) to continue or Right(result) to finish.
// Stack-safe: delegates recursion to ExprLoop's iterative trampoline via
// Reify/Reflect, avoiding Go stack growth on deep pure Left chains.
func Loop[S, A any](initial S, step func(S) kont.Eff[kont.Either[S, A]]) kont.Eff[A] {
	return kont.Suspend(func(k func(A) kont.Resumed) kont.Resumed {
		loopExpr := ExprLoop[S, A](initial, func(s S) kont.Expr[kont.Either[S, A]] {
			return Reify(step(s))
		})
		return Reflect(loopExpr)(k)
	})
}

// ExprLoop runs a recursive session protocol (Expr-world).
// Left supplies the next state. Right supplies the final result.
// Completed steps iterate without growing the Go stack. If a step returns a
// deferred expression, build a fresh loop for each run.
func ExprLoop[S, A any](initial S, step func(S) kont.Expr[kont.Either[S, A]]) kont.Expr[A] {
	s := initial
	for {
		m := step(s)
		if _, ok := m.Frame.(kont.ReturnFrame); ok {
			if left, ok := m.Value.GetLeft(); ok {
				s = left
				continue
			}
			right, _ := m.Value.GetRight()
			return kont.ExprReturn(right)
		}
		return kont.ExprSuspend[A](enterExpr(m, loopFrame[S, A]{step: step}))
	}
}

type loopFrame[S, A any] struct {
	kont.ReturnFrame
	step func(S) kont.Expr[kont.Either[S, A]]
}

func (f loopFrame[S, A]) Unwind(v kont.Erased) (kont.Erased, kont.Frame) {
	e := resultOrZero[kont.Either[S, A]](v)
	if left, ok := e.GetLeft(); ok {
		next := ExprLoop(left, f.step)
		return frameInput(&next), next.Frame
	}
	right, _ := e.GetRight()
	return right, kont.ReturnFrame{}
}

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

func exprLoopUnwind[S, A any](data, _, _, current kont.Erased) (kont.Erased, kont.Frame) {
	step := data.(func(S) kont.Expr[kont.Either[S, A]])
	e := current.(kont.Either[S, A])
	if left, ok := e.GetLeft(); ok {
		return exprLoopIter[S, A](left, step)
	}
	right, _ := e.GetRight()
	return kont.Erased(right), kont.ReturnFrame{}
}

// exprLoopIter is the iterative core shared by ExprLoop and exprLoopUnwind.
// It loops over pure completed steps without growing the Go stack, and returns
// a frame chain when the step suspends on an effect.
func exprLoopIter[S, A any](s S, step func(S) kont.Expr[kont.Either[S, A]]) (kont.Erased, kont.Frame) {
	for {
		m := step(s)
		if _, ok := m.Frame.(kont.ReturnFrame); ok {
			if left, ok := m.Value.GetLeft(); ok {
				s = left
				continue
			}
			right, _ := m.Value.GetRight()
			return kont.Erased(right), kont.ReturnFrame{}
		}
		bf := kont.AcquireUnwindFrame()
		bf.Data1 = step
		bf.Unwind = exprLoopUnwind[S, A]
		return kont.Erased(nil), kont.ChainFrames(m.Frame, bf)
	}
}

// ExprLoop runs a recursive session protocol (Expr-world).
// step returns Left(nextState) to continue or Right(result) to finish.
// Stack-safe: pure completed steps are iterated without Go stack growth;
// effectful steps are trampolined through evalFrames via UnwindFrame.
func ExprLoop[S, A any](initial S, step func(S) kont.Expr[kont.Either[S, A]]) kont.Expr[A] {
	value, frame := exprLoopIter[S, A](initial, step)
	var zero A
	if _, ok := frame.(kont.ReturnFrame); ok {
		return kont.ExprReturn(value.(A))
	}
	return kont.Expr[A]{
		Value: zero,
		Frame: frame,
	}
}

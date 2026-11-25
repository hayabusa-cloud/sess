// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess

import (
	"code.hybscloud.com/kont"
)

// Reify converts a Cont-world session protocol to Expr-world.
// The resulting Expr can be evaluated with ExecExpr, RunExpr,
// or stepped with Step and Advance.
func Reify[A any](m kont.Eff[A]) kont.Expr[A] {
	return kont.Reify(m)
}

// Reflect converts an Expr-world session protocol to Cont-world.
// The resulting Eff can be evaluated with Exec or Run.
func Reflect[A any](m kont.Expr[A]) kont.Eff[A] {
	return kont.Reflect(m)
}

// enterExpr preserves m's first frame input when the enclosing expression
// has a different result type. next receives m's final value.
func enterExpr[A any](m kont.Expr[A], next kont.Frame) kont.Frame {
	first := kont.AcquireThenFrame()
	first.Second = kont.Expr[kont.Erased]{Frame: m.Frame}
	// An EffectFrame resumes with the operation's reply.
	if _, effect := m.Frame.(*kont.EffectFrame[kont.Erased]); !effect {
		first.Second.Value = m.Value
	}
	first.Next = next
	return first
}

// A bare nil at a kont result boundary represents the zero value of A.
func resultOrZero[A any](v kont.Erased) A {
	if v == nil {
		var zero A
		return zero
	}
	return v.(A)
}

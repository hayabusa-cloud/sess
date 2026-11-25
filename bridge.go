// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess

import (
	"code.hybscloud.com/kont"
)

// Reify runs a protocol to its first return or effect. A completed result is
// reusable. A pending result and its copies share one invocation. Reify a
// reusable protocol again to start another run.
func Reify[A any](m kont.Eff[A]) kont.Expr[A] {
	return kont.Reify(m)
}

// Reflect converts an expression to a Cont-world protocol for [Exec] or [Run].
// The protocol can be run again only if the input expression is reusable.
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

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

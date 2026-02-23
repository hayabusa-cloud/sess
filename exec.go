// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess

import (
	"code.hybscloud.com/kont"
)

// Exec runs a Cont-world session protocol on a pre-created endpoint.
// Blocks on iox.ErrWouldBlock via adaptive backoff (iox.Backoff),
// without spawning goroutines or creating channels.
func Exec[R any](ep *Endpoint, protocol kont.Eff[R]) R {
	h := sessionHandler[R]{ctx: &ep.ctx}
	return kont.Handle(protocol, h)
}

// ExecExpr runs an Expr-world session protocol on a pre-created endpoint.
// Blocks on iox.ErrWouldBlock via adaptive backoff (iox.Backoff),
// without spawning goroutines or creating channels.
func ExecExpr[R any](ep *Endpoint, protocol kont.Expr[R]) R {
	h := sessionHandler[R]{ctx: &ep.ctx}
	return kont.HandleExpr(protocol, h)
}

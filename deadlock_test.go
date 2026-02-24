// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess_test

import (
	"testing"
	"time"

	"code.hybscloud.com/kont"
	"code.hybscloud.com/sess"
)

func TestRunExprDeadlockCoverage(t *testing.T) {
	a := sess.ExprRecvBind(func(n int) kont.Expr[struct{}] { return sess.ExprCloseDone(struct{}{}) })
	b := sess.ExprRecvBind(func(n int) kont.Expr[struct{}] { return sess.ExprCloseDone(struct{}{}) })

	go func() {
		sess.RunExpr[struct{}, struct{}](a, b)
	}()

	time.Sleep(50 * time.Millisecond) // Give it time to hit bo.Wait()
}

func TestRunErrorExprDeadlockCoverage(t *testing.T) {
	a := sess.ExprRecvBind(func(n int) kont.Expr[struct{}] { return sess.ExprCloseDone(struct{}{}) })
	b := sess.ExprRecvBind(func(n int) kont.Expr[struct{}] { return sess.ExprCloseDone(struct{}{}) })

	go func() {
		sess.RunErrorExpr[string, struct{}, struct{}](a, b)
	}()

	time.Sleep(50 * time.Millisecond) // Give it time to hit bo.Wait()
}

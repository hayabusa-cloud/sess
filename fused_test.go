// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess_test

import (
	"fmt"
	"testing"

	"code.hybscloud.com/kont"
	"code.hybscloud.com/sess"
)

func TestSendThen(t *testing.T) {
	skipRace(t)
	client := sess.SendThen(42, sess.CloseDone("sent"))

	server := sess.RecvBind(func(n int) kont.Eff[string] {
		return sess.CloseDone(fmt.Sprintf("got %d", n))
	})

	clientResult, serverResult := sess.Run[string, string](client, server)
	if clientResult != "sent" {
		t.Fatalf("client got %q, want %q", clientResult, "sent")
	}
	if serverResult != "got 42" {
		t.Fatalf("server got %q, want %q", serverResult, "got 42")
	}
}

func TestRecvBind(t *testing.T) {
	skipRace(t)
	client := sess.SendThen(99, sess.CloseDone("done"))

	server := sess.RecvBind(func(n int) kont.Eff[int] {
		return sess.CloseDone(n * 2)
	})

	_, serverResult := sess.Run[string, int](client, server)
	if serverResult != 198 {
		t.Fatalf("server got %d, want 198", serverResult)
	}
}

func TestExprSendThen(t *testing.T) {
	skipRace(t)
	client := sess.ExprSendThen(42, sess.ExprCloseDone("sent"))

	server := sess.ExprRecvBind(func(n int) kont.Expr[string] {
		return sess.ExprCloseDone(fmt.Sprintf("got %d", n))
	})

	clientResult, serverResult := sess.RunExpr[string, string](client, server)
	if clientResult != "sent" {
		t.Fatalf("client got %q, want %q", clientResult, "sent")
	}
	if serverResult != "got 42" {
		t.Fatalf("server got %q, want %q", serverResult, "got 42")
	}
}

func TestExprRecvBind(t *testing.T) {
	skipRace(t)
	client := sess.ExprSendThen(99, sess.ExprCloseDone("done"))

	server := sess.ExprRecvBind(func(n int) kont.Expr[int] {
		return sess.ExprCloseDone(n * 2)
	})

	_, serverResult := sess.RunExpr[string, int](client, server)
	if serverResult != 198 {
		t.Fatalf("server got %d, want 198", serverResult)
	}
}

func TestFusedProtocol(t *testing.T) {
	skipRace(t)
	// Full protocol using only fused API
	client := sess.SendThen(100,
		sess.SendThen("hello",
			sess.RecvBind(func(n int) kont.Eff[int] {
				return sess.CloseDone(n)
			}),
		),
	)

	server := sess.RecvBind(func(n int) kont.Eff[string] {
		return sess.RecvBind(func(s string) kont.Eff[string] {
			return sess.SendThen(n*2,
				sess.CloseDone(fmt.Sprintf("%s:%d", s, n)),
			)
		})
	})

	clientResult, serverResult := sess.Run[int, string](client, server)
	if clientResult != 200 {
		t.Fatalf("client got %d, want 200", clientResult)
	}
	if serverResult != "hello:100" {
		t.Fatalf("server got %q, want %q", serverResult, "hello:100")
	}
}

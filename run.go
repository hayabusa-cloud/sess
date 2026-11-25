// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess

import (
	"code.hybscloud.com/iox"
	"code.hybscloud.com/kont"
)

// Run creates a session pair, runs both Cont-world protocols, and returns
// both results. Interleaves execution of both sides on the calling
// goroutine using adaptive backoff (iox.Backoff) when neither side
// can make progress. Does not spawn goroutines or create channels.
func Run[A, B any](a kont.Eff[A], b kont.Eff[B]) (A, B) {
	resultA, suspA := kont.Step(a)
	resultB, suspB := kont.Step(b)
	epA, epB := New()
	return runPair(epA, epB, resultA, suspA, resultB, suspB)
}

// RunExpr creates a session pair, runs both Expr-world protocols, and
// returns both results. Interleaves execution of both sides on the
// calling goroutine using adaptive backoff (iox.Backoff) when neither
// side can make progress. Does not spawn goroutines or create channels.
func RunExpr[A, B any](a kont.Expr[A], b kont.Expr[B]) (A, B) {
	epA, epB := New()
	resultA, suspA := Step[A](a)
	resultB, suspB := Step[B](b)
	return runPair(epA, epB, resultA, suspA, resultB, suspB)
}

func runPair[A, B any](epA, epB *Endpoint, resultA A, suspA *kont.Suspension[A], resultB B, suspB *kont.Suspension[B]) (A, B) {
	var bo iox.Backoff

	sopA := pendingDispatcher(suspA)
	sopB := pendingDispatcher(suspB)

	for suspA != nil || suspB != nil {
		progress := false
		if suspA != nil {
			if v, ok := runExprDispatchSession(&epA.ctx, sopA); ok {
				resultA, suspA = suspA.Resume(v)
				sopA = pendingDispatcher(suspA)
				progress = true
			}
		}
		if suspB != nil {
			if v, ok := runExprDispatchSession(&epB.ctx, sopB); ok {
				resultB, suspB = suspB.Resume(v)
				sopB = pendingDispatcher(suspB)
				progress = true
			}
		}
		if !progress {
			bo.Wait()
		} else {
			bo.Reset()
		}
	}
	return resultA, resultB
}

// A completed role can release its last operation while its peer continues.
func pendingDispatcher[R any](susp *kont.Suspension[R]) sessionDispatcher {
	if susp == nil {
		return nil
	}
	sop, ok := susp.Op().(sessionDispatcher)
	if !ok {
		panic("sess: unhandled effect in RunExpr")
	}
	return sop
}

func runExprDispatchSession(ctx *sessionContext, sop sessionDispatcher) (kont.Resumed, bool) {
	v, err := sop.DispatchSession(ctx)
	if err == nil {
		return v, true
	}
	requireWouldBlock(err)
	return nil, false
}

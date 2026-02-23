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
	return RunExpr(Reify(a), Reify(b))
}

// RunExpr creates a session pair, runs both Expr-world protocols, and
// returns both results. Interleaves execution of both sides on the
// calling goroutine using adaptive backoff (iox.Backoff) when neither
// side can make progress. Does not spawn goroutines or create channels.
func RunExpr[A, B any](a kont.Expr[A], b kont.Expr[B]) (A, B) {
	epA, epB := New()
	resultA, suspA := Step[A](a)
	resultB, suspB := Step[B](b)
	var bo iox.Backoff

	var sopA sessionDispatcher
	if suspA != nil {
		sopA = suspA.Op().(sessionDispatcher)
	}
	var sopB sessionDispatcher
	if suspB != nil {
		sopB = suspB.Op().(sessionDispatcher)
	}

	for suspA != nil || suspB != nil {
		progress := false
		if suspA != nil {
			v, err := sopA.DispatchSession(&epA.ctx)
			if err == nil {
				resultA, suspA = suspA.Resume(v)
				if suspA != nil {
					sopA = suspA.Op().(sessionDispatcher)
				}
				progress = true
			}
		}
		if suspB != nil {
			v, err := sopB.DispatchSession(&epB.ctx)
			if err == nil {
				resultB, suspB = suspB.Resume(v)
				if suspB != nil {
					sopB = suspB.Op().(sessionDispatcher)
				}
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

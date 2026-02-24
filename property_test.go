// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess_test

import (
	"reflect"
	"testing"
	"testing/quick"

	"code.hybscloud.com/kont"
	"code.hybscloud.com/sess"
)

// TestPropertyTransportFIFO proves that for any arbitrarily generated sequence
// of integers, the session transport guarantees strict FIFO delivery without
// loss, duplication, or reordering.
func TestPropertyTransportFIFO(t *testing.T) {
	skipRace(t)

	// The property function receives an arbitrary slice of integers.
	propertyFIFO := func(payload []int) bool {
		// Sender: iterates through the payload, sending each element.
		sender := sess.Loop(payload, func(s []int) kont.Eff[kont.Either[[]int, struct{}]] {
			if len(s) == 0 {
				// Signal end of payload
				return sess.SelectRThen(sess.CloseDone(kont.Right[[]int, struct{}](struct{}{})))
			}
			// Signal more data and send it
			return sess.SelectLThen(
				sess.SendThen(s[0], kont.Pure(kont.Left[[]int, struct{}](s[1:]))),
			)
		})

		// Receiver: collects elements until the sender signals end of payload.
		receiver := sess.Loop(make([]int, 0, len(payload)), func(acc []int) kont.Eff[kont.Either[[]int, []int]] {
			return sess.OfferBranch(
				// Left branch: more data
				func() kont.Eff[kont.Either[[]int, []int]] {
					return sess.RecvBind(func(n int) kont.Eff[kont.Either[[]int, []int]] {
						return kont.Pure(kont.Left[[]int, []int](append(acc, n)))
					})
				},
				// Right branch: end of payload
				func() kont.Eff[kont.Either[[]int, []int]] {
					return sess.CloseDone(kont.Right[[]int, []int](acc))
				},
			)
		})

		// Run the session pair.
		_, received := sess.Run[struct{}, []int](sender, receiver)

		// Verification: the received sequence must exactly match the sent payload.
		// Use reflect.DeepEqual to correctly handle empty vs nil slices.
		if len(payload) == 0 && len(received) == 0 {
			return true
		}
		return reflect.DeepEqual(payload, received)
	}

	// testing/quick generates arbitrary slices and checks the property.
	if err := quick.Check(propertyFIFO, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyErrorShortCircuit proves that an error thrown at any arbitrary point
// in a session protocol always cleanly short-circuits the session and returns
// the exact error value as the Left branch of the Either result.
func TestPropertyErrorShortCircuit(t *testing.T) {
	skipRace(t)

	propertyError := func(throwAt uint) bool {
		throwMsg := "forced_error"
		n := throwAt % 3

		sender := sess.ExprLoop(uint(0), func(i uint) kont.Expr[kont.Either[uint, string]] {
			if i == n {
				// Eager error short-circuit: map ThrowError to the expected type
				throwEff := kont.ThrowError[string, string](throwMsg)
				mappedThrow := kont.Map(throwEff, func(s string) kont.Either[uint, string] {
					return kont.Right[uint, string](s)
				})
				return sess.Reify(mappedThrow)
			}
			return sess.ExprSendThen(i, kont.ExprReturn(kont.Left[uint, string](i+1)))
		})

		// evaluate using StepError and AdvanceError until completion or suspension
		result, susp := sess.StepError[string, string](sender)

		// advance n times
		ep, _ := sess.New()

		for susp != nil {
			var err error
			result, susp, err = sess.AdvanceError[string](ep, susp)
			if err != nil {
				// queue full, just retry
				continue
			}
		}

		errVal, isErr := result.GetLeft()
		return isErr && errVal == throwMsg
	}

	if err := quick.Check(propertyError, nil); err != nil {
		t.Error(err)
	}
}

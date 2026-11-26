// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess_test

import (
	"bytes"
	"testing"

	"code.hybscloud.com/iox"
	"code.hybscloud.com/kont"
	"code.hybscloud.com/sess"
)

// BenchmarkSendRetry measures one refused pointer send on an already full queue.
func BenchmarkSendRetry(b *testing.B) {
	value := 42
	benchmarkSendRetry(b, &value)
}

func BenchmarkSendRetryBytes(b *testing.B) {
	benchmarkSendRetry(b, []byte("session payload"))
}

func BenchmarkSendRetryBytesPreboxed(b *testing.B) {
	benchmarkSendRetry[any](b, []byte("session payload"))
}

func BenchmarkSendRetryValue32(b *testing.B) {
	benchmarkSendRetry(b, [4]uint64{1, 2, 3, 4})
}

func benchmarkSendRetry[T any](b *testing.B, value T) {
	sender, _ := sess.New()
	var pending *kont.Suspension[struct{}]
	for pending == nil {
		_, h := sess.Step(kont.ExprPerform(sess.Send[T]{Value: value}))
		_, next, err := sess.Advance(sender, h)
		if err == nil && next == nil {
			continue
		}
		if next != h || !iox.IsWouldBlock(err) {
			b.Fatalf("fill: next=%v, err=%v", next, err)
		}
		pending = h
	}
	defer pending.Discard()
	b.ReportAllocs()
	for b.Loop() {
		_, next, err := sess.Advance(sender, pending)
		if next != pending || !iox.IsWouldBlock(err) {
			b.Fatalf("retry: next=%v, err=%v", next, err)
		}
	}
}

// BenchmarkSendDelivery measures a successful send and receive on existing endpoints.
func BenchmarkSendDelivery(b *testing.B) {
	b.Run("Pointer", func(b *testing.B) {
		value := 42
		if got := benchmarkSendDelivery(b, &value); got != &value {
			b.Fatal("received a different pointer")
		}
	})
	b.Run("Bytes", func(b *testing.B) {
		value := []byte("session payload")
		if got := benchmarkSendDelivery(b, value); !bytes.Equal(got, value) {
			b.Fatalf("received %q, want %q", got, value)
		}
	})
	b.Run("Value32", func(b *testing.B) {
		value := [4]uint64{1, 2, 3, 4}
		if got := benchmarkSendDelivery(b, value); got != value {
			b.Fatalf("received %v, want %v", got, value)
		}
	})
	b.Run("FreshBytes/Typed", func(b *testing.B) {
		benchmarkFreshByteSend(b, func(v []byte) kont.Expr[struct{}] {
			return kont.ExprPerform(sess.Send[[]byte]{Value: v})
		})
	})
	b.Run("FreshBytes/Preboxed", func(b *testing.B) {
		benchmarkFreshByteSend(b, func(v []byte) kont.Expr[struct{}] {
			return kont.ExprPerform(sess.Send[any]{Value: v})
		})
	})
}

func benchmarkSendDelivery[T any](b *testing.B, value T) T {
	sender, receiver := sess.New()
	send := kont.ExprPerform(sess.Send[T]{Value: value})
	recv := kont.ExprPerform(sess.Recv[T]{})
	var got T
	b.ReportAllocs()
	for b.Loop() {
		got = benchmarkDeliver(b, sender, receiver, send, recv)
	}
	return got
}

func benchmarkFreshByteSend(b *testing.B, send func([]byte) kont.Expr[struct{}]) {
	sender, receiver := sess.New()
	recv := kont.ExprPerform(sess.Recv[[]byte]{})
	data := []byte("session payload")
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		value := data[i&1:]
		got := benchmarkDeliver(b, sender, receiver, send(value), recv)
		if len(got) != len(value) || &got[0] != &value[0] {
			b.Fatal("received a different byte slice")
		}
	}
}

func benchmarkDeliver[T any](b *testing.B, sender, receiver *sess.Endpoint, send kont.Expr[struct{}], recv kont.Expr[T]) T {
	_, h := sess.Step(send)
	if _, next, err := sess.Advance(sender, h); next != nil || err != nil {
		b.Fatalf("send: next=%v, err=%v", next, err)
	}
	_, pending := sess.Step(recv)
	value, next, err := sess.Advance(receiver, pending)
	if next != nil || err != nil {
		b.Fatalf("receive: next=%v, err=%v", next, err)
	}
	return value
}

// BenchmarkChannelSendRetry measures a refused send on a full buffered channel.
func BenchmarkChannelSendRetry(b *testing.B) {
	ch := make(chan *int, 4)
	value := 42
	for range cap(ch) {
		ch <- &value
	}
	b.ReportAllocs()
	for b.Loop() {
		select {
		case ch <- &value:
			b.Fatal("send on full channel succeeded")
		default:
		}
	}
}

// BenchmarkSendRecv measures a single send/recv round-trip.
func BenchmarkSendRecv(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sender := sess.SendThen(42, sess.CloseDone(struct{}{}))
		receiver := sess.RecvBind(func(n int) kont.Eff[int] {
			return sess.CloseDone(n)
		})
		sess.Run[struct{}, int](sender, receiver)
	}
}

// BenchmarkProtocol3Step measures a 3-step protocol (send, recv, close).
func BenchmarkProtocol3Step(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		client := sess.SendThen(1,
			sess.RecvBind(func(n int) kont.Eff[int] {
				return sess.CloseDone(n)
			}),
		)
		server := sess.RecvBind(func(n int) kont.Eff[int] {
			return sess.SendThen(n*2, sess.CloseDone(n*2))
		})
		sess.Run[int, int](client, server)
	}
}

// BenchmarkSelectOffer measures select/offer branch round-trip.
func BenchmarkSelectOffer(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		selector := sess.SelectLThen(sess.CloseDone(struct{}{}))
		offerer := sess.OfferBranch(
			func() kont.Eff[struct{}] { return sess.CloseDone(struct{}{}) },
			func() kont.Eff[struct{}] { return sess.CloseDone(struct{}{}) },
		)
		sess.Run[struct{}, struct{}](selector, offerer)
	}
}

// BenchmarkExprSelectOffer measures Expr-world select/offer branch round-trip.
func BenchmarkExprSelectOffer(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		selector := sess.ExprSelectLThen(sess.ExprCloseDone(struct{}{}))
		offerer := sess.ExprOfferBranch(
			func() kont.Expr[struct{}] { return sess.ExprCloseDone(struct{}{}) },
			func() kont.Expr[struct{}] { return sess.ExprCloseDone(struct{}{}) },
		)
		sess.RunExpr[struct{}, struct{}](selector, offerer)
	}
}

// BenchmarkExprSendRecv measures Expr-world send/recv round-trip.
func BenchmarkExprSendRecv(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sender := sess.ExprSendThen(42, sess.ExprCloseDone(struct{}{}))
		receiver := sess.ExprRecvBind(func(n int) kont.Expr[int] {
			return sess.ExprCloseDone(n)
		})
		sess.RunExpr[struct{}, int](sender, receiver)
	}
}

// BenchmarkExprSendRecvStructResult returns a four-counter summary from the sender.
func BenchmarkExprSendRecvStructResult(b *testing.B) {
	type counts struct{ Sent, Received, SentBytes, ReceivedBytes int64 }
	sent := counts{Sent: 1, SentBytes: 8}
	b.ReportAllocs()
	for b.Loop() {
		sender := sess.ExprSendThen(int64(42), sess.ExprCloseDone(sent))
		receiver := sess.ExprRecvBind(sess.ExprCloseDone[int64])
		result, value := sess.RunExpr(sender, receiver)
		if result != sent || value != 42 {
			b.Fatal(result, value)
		}
	}
}

// BenchmarkExprProtocol3Step measures Expr-world 3-step protocol.
func BenchmarkExprProtocol3Step(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		client := sess.ExprSendThen(1,
			sess.ExprRecvBind(func(n int) kont.Expr[int] {
				return sess.ExprCloseDone(n)
			}),
		)
		server := sess.ExprRecvBind(func(n int) kont.Expr[int] {
			return sess.ExprSendThen(n*2, sess.ExprCloseDone(n*2))
		})
		sess.RunExpr[int, int](client, server)
	}
}

// BenchmarkDelegation measures endpoint delegation (send endpoint to peer).
func BenchmarkDelegation(b *testing.B) {
	skipRace(b)
	b.ReportAllocs()
	for b.Loop() {
		subA, subB := sess.New()
		done := make(chan struct{})
		go func() {
			sess.Exec(subB, sess.RecvBind(func(s string) kont.Eff[string] {
				return sess.CloseDone(s)
			}))
			close(done)
		}()
		delegator := sess.SendThen(subA, sess.CloseDone(struct{}{}))
		acceptor := sess.RecvBind(func(ep *sess.Endpoint) kont.Eff[struct{}] {
			sess.Exec(ep, sess.SendThen("hello", sess.CloseDone(struct{}{})))
			return sess.CloseDone(struct{}{})
		})
		sess.Run[struct{}, struct{}](delegator, acceptor)
		<-done
	}
}

// BenchmarkRecLoop measures recursive session protocol via Loop.
func BenchmarkRecLoop(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		client := sess.Loop(0, func(i int) kont.Eff[kont.Either[int, struct{}]] {
			if i >= 5 {
				return kont.Bind(sess.CloseDone(struct{}{}), func(_ struct{}) kont.Eff[kont.Either[int, struct{}]] {
					return kont.Pure(kont.Right[int, struct{}](struct{}{}))
				})
			}
			return kont.Bind(sess.SendThen(i, kont.Pure(struct{}{})), func(_ struct{}) kont.Eff[kont.Either[int, struct{}]] {
				return kont.Pure(kont.Left[int, struct{}](i + 1))
			})
		})
		server := sess.RecvBind(func(_ int) kont.Eff[int] {
			return sess.RecvBind(func(_ int) kont.Eff[int] {
				return sess.RecvBind(func(_ int) kont.Eff[int] {
					return sess.RecvBind(func(_ int) kont.Eff[int] {
						return sess.RecvBind(func(n int) kont.Eff[int] {
							return sess.CloseDone(n)
						})
					})
				})
			})
		})
		sess.Run[struct{}, int](client, server)
	}
}

// BenchmarkExprRecLoop measures Expr-world recursive session protocol via ExprLoop.
func BenchmarkExprRecLoop(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		client := sess.ExprLoop(0, func(i int) kont.Expr[kont.Either[int, struct{}]] {
			if i >= 5 {
				return sess.ExprCloseDone(kont.Right[int, struct{}](struct{}{}))
			}
			return sess.ExprSendThen(i, kont.ExprReturn(kont.Left[int, struct{}](i+1)))
		})
		server := sess.ExprRecvBind(func(_ int) kont.Expr[int] {
			return sess.ExprRecvBind(func(_ int) kont.Expr[int] {
				return sess.ExprRecvBind(func(_ int) kont.Expr[int] {
					return sess.ExprRecvBind(func(_ int) kont.Expr[int] {
						return sess.ExprRecvBind(func(n int) kont.Expr[int] {
							return sess.ExprCloseDone(n)
						})
					})
				})
			})
		})
		sess.RunExpr[struct{}, int](client, server)
	}
}

// BenchmarkExec measures single-endpoint Exec convenience path.
func BenchmarkExec(b *testing.B) {
	skipRace(b)
	b.ReportAllocs()
	for b.Loop() {
		epA, epB := sess.New()
		done := make(chan struct{})
		go func() {
			sess.Exec(epB, sess.RecvBind(func(n int) kont.Eff[int] {
				return sess.CloseDone(n)
			}))
			close(done)
		}()
		sess.Exec(epA, sess.SendThen(42, sess.CloseDone(struct{}{})))
		<-done
	}
}

// BenchmarkErrorPath measures RunError with error handler dispatch.
func BenchmarkErrorPath(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		client := kont.Bind(
			kont.CatchError(
				kont.ThrowError[string, string]("err"),
				func(e string) kont.Eff[string] {
					return kont.Pure("recovered")
				},
			),
			func(s string) kont.Eff[string] {
				return sess.SendThen(s, sess.CloseDone(s))
			},
		)
		server := sess.RecvBind(func(s string) kont.Eff[string] {
			return sess.CloseDone(s)
		})
		sess.RunError[string, string, string](client, server)
	}
}

// BenchmarkStepAdvance measures stepping a protocol via Step+Advance.
func BenchmarkStepAdvance(b *testing.B) {
	skipRace(b)
	b.ReportAllocs()
	for b.Loop() {
		epA, epB := sess.New()
		sender := sess.ExprSendThen(42, sess.ExprCloseDone(struct{}{}))
		receiver := sess.ExprRecvBind(func(n int) kont.Expr[int] {
			return sess.ExprCloseDone(n)
		})

		done := make(chan struct{})
		go func() {
			result, susp := sess.Step[struct{}](sender)
			_ = result
			for susp != nil {
				result, susp, _ = sess.Advance(epA, susp)
			}
			close(done)
		}()

		result, susp := sess.Step[int](receiver)
		for susp != nil {
			var err error
			result, susp, err = sess.Advance(epB, susp)
			if err != nil {
				continue
			}
		}
		<-done
		_ = result
	}
}

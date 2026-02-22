// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess

import (
	"code.hybscloud.com/kont"
)

// Send is the effect operation for sending a value of type T.
// Perform(Send[T]{Value: v}) sends v to the peer endpoint.
type Send[T any] struct {
	kont.Phantom[struct{}]
	Value T
}

// DispatchSession handles Send on the session transport.
// Non-blocking: returns iox.ErrWouldBlock if the bounded SPSC queue is full.
func (s Send[T]) DispatchSession(ctx *sessionContext) (kont.Resumed, error) {
	ctx.sendSlot = s.Value
	if err := ctx.sendQ.Enqueue(&ctx.sendSlot); err != nil {
		return nil, err
	}
	return struct{}{}, nil
}

// Recv is the effect operation for receiving a value of type T.
// Perform(Recv[T]{}) receives a typed value from the peer.
type Recv[T any] struct {
	kont.Phantom[T]
}

// DispatchSession handles Recv on the session transport.
// Non-blocking: returns iox.ErrWouldBlock if the bounded SPSC queue is empty.
func (Recv[T]) DispatchSession(ctx *sessionContext) (kont.Resumed, error) {
	v, err := ctx.recvQ.Dequeue()
	if err != nil {
		return nil, err
	}
	return v.(T), nil
}

// Close is the effect operation for closing the session.
// Perform(Close{}) signals session termination.
type Close struct {
	kont.Phantom[struct{}]
}

// DispatchSession handles Close on the session transport.
// Atomically increments the shared close counter. Never blocks.
func (Close) DispatchSession(ctx *sessionContext) (kont.Resumed, error) {
	ctx.closed.Add(1)
	return struct{}{}, nil
}

// signalLeft and signalRight are pre-allocated choice values
// for SelectL/SelectR, avoiding per-dispatch heap escape.
var (
	signalLeft  = true
	signalRight = false
)

// offerLeft and offerRight are pre-boxed Resumed values for Offer dispatch.
// Either[struct{}, struct{}] is non-zero-size (contains isRight bool),
// so boxing into Resumed (any) allocates without pre-allocation.
var (
	offerLeft  kont.Resumed = kont.Left[struct{}, struct{}](struct{}{})
	offerRight kont.Resumed = kont.Right[struct{}](struct{}{})
)

// SelectL is the effect operation for choosing the left branch.
// Perform(SelectL{}) signals the left choice to the peer.
type SelectL struct {
	kont.Phantom[struct{}]
}

// DispatchSession handles SelectL on the session transport.
// Non-blocking: returns iox.ErrWouldBlock if the choice queue is full.
func (SelectL) DispatchSession(ctx *sessionContext) (kont.Resumed, error) {
	if err := ctx.signalQ.Enqueue(&signalLeft); err != nil {
		return nil, err
	}
	return struct{}{}, nil
}

// SelectR is the effect operation for choosing the right branch.
// Perform(SelectR{}) signals the right choice to the peer.
type SelectR struct {
	kont.Phantom[struct{}]
}

// DispatchSession handles SelectR on the session transport.
// Non-blocking: returns iox.ErrWouldBlock if the choice queue is full.
func (SelectR) DispatchSession(ctx *sessionContext) (kont.Resumed, error) {
	if err := ctx.signalQ.Enqueue(&signalRight); err != nil {
		return nil, err
	}
	return struct{}{}, nil
}

// Offer is the effect operation for receiving a branch choice from the peer.
// Perform(Offer{}) receives the peer's Left or Right selection.
type Offer struct {
	kont.Phantom[kont.Either[struct{}, struct{}]]
}

// DispatchSession handles Offer on the session transport.
// Non-blocking: returns iox.ErrWouldBlock if the choice queue is empty.
// true → Left (peer selected left), false → Right (peer selected right).
func (Offer) DispatchSession(ctx *sessionContext) (kont.Resumed, error) {
	v, err := ctx.awaitQ.Dequeue()
	if err != nil {
		return nil, err
	}
	if v {
		return offerLeft, nil
	}
	return offerRight, nil
}

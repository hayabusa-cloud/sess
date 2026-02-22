// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess

import (
	"code.hybscloud.com/atomix"
	"code.hybscloud.com/iox"
	"code.hybscloud.com/kont"
	"code.hybscloud.com/lfq"
)

// channelCapacity is the bounded capacity for session transport queues.
// 4 balances amortizing producer-side cached-index refresh cost while
// keeping ring buffers within a single cache line.
const channelCapacity = 4

// sessionContext holds the lock-free transport for a single endpoint.
// Each direction is a single-producer single-consumer bounded queue.
type sessionContext struct {
	sendQ    *lfq.SPSC[any]
	recvQ    *lfq.SPSC[any]
	signalQ  *lfq.SPSC[bool]
	awaitQ   *lfq.SPSC[bool]
	closed   *atomix.Uint32
	sendSlot any
}

// sessionDispatcher is the structural interface for session operations.
// DispatchSession is non-blocking: it returns iox.ErrWouldBlock at
// the I/O boundary when the bounded queue cannot make progress.
type sessionDispatcher interface {
	DispatchSession(ctx *sessionContext) (kont.Resumed, error)
}

// sessionHandler implements kont.Handler for session effects.
// Waits on iox.ErrWouldBlock, converting non-blocking dispatch
// into blocking evaluation for Exec/ExecExpr.
// Value type: passed to evalFrames on the stack, avoiding heap allocation.
type sessionHandler[R any] struct {
	ctx *sessionContext
}

// Dispatch implements kont.Handler via structural interface assertion.
// Waits past the iox.ErrWouldBlock boundary with adaptive backoff.
func (h sessionHandler[R]) Dispatch(op kont.Operation) (kont.Resumed, bool) {
	sop, ok := op.(sessionDispatcher)
	if !ok {
		panic("sess: unhandled effect in sessionHandler")
	}
	return dispatchWait(h.ctx, sop), true
}

// dispatchWait blocks until DispatchSession succeeds, backing off on
// iox.ErrWouldBlock with iox.Backoff (I/O readiness waiting).
func dispatchWait(ctx *sessionContext, sop sessionDispatcher) kont.Resumed {
	var bo iox.Backoff
	for {
		v, err := sop.DispatchSession(ctx)
		if err == nil {
			return v
		}
		bo.Wait()
	}
}

// Endpoint represents one side of a session-typed channel pair.
// Transport is backed by bounded lock-free SPSC queues from lfq.
type Endpoint struct {
	ctx    sessionContext
	serial Serial
}

// Serial returns the serial number assigned to this endpoint's session.
func (ep *Endpoint) Serial() Serial {
	return ep.serial
}

// endpointPair holds both endpoints, queues, and shared state
// in a single allocation. SPSC queues are embedded as values;
// only the ring buffers are separate heap objects.
type endpointPair struct {
	a        Endpoint
	b        Endpoint
	closed   atomix.Uint32
	dataAB   lfq.SPSC[any]
	dataBA   lfq.SPSC[any]
	choiceAB lfq.SPSC[bool]
	choiceBA lfq.SPSC[bool]
}

// New creates a connected pair of session endpoints.
// Internal transport uses bounded lock-free SPSC queues: two for data
// (A→B, B→A), two for branch choice (A→B, B→A), and a shared atomic
// counter for close signaling.
//
// Session operations are non-blocking: DispatchSession returns
// iox.ErrWouldBlock when the peer has not yet produced or consumed.
func New() (*Endpoint, *Endpoint) {
	s := nextSerial()

	pair := &endpointPair{}
	pair.dataAB.Init(channelCapacity)
	pair.dataBA.Init(channelCapacity)
	pair.choiceAB.Init(channelCapacity)
	pair.choiceBA.Init(channelCapacity)

	pair.a = Endpoint{
		ctx: sessionContext{
			sendQ:   &pair.dataAB,
			recvQ:   &pair.dataBA,
			signalQ: &pair.choiceAB,
			awaitQ:  &pair.choiceBA,
			closed:  &pair.closed,
		},
		serial: s,
	}
	pair.b = Endpoint{
		ctx: sessionContext{
			sendQ:   &pair.dataBA,
			recvQ:   &pair.dataAB,
			signalQ: &pair.choiceBA,
			awaitQ:  &pair.choiceAB,
			closed:  &pair.closed,
		},
		serial: s,
	}
	return &pair.a, &pair.b
}

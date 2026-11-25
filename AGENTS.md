# sess

In `sess`, a local protocol describes one role’s actions at an endpoint. `New` creates two peer endpoints connected by bounded in-memory queues. Go checks each operation’s payload and reply types. The caller supplies dual local protocols and gives each endpoint one owner at a time.

- `Exec` and `ExecExpr` drive one role on an existing endpoint. `Run` and `RunExpr` create an endpoint pair and drive both roles on the calling goroutine. Check the third result (`thrown`) from `RunError` or `RunErrorExpr` first. If it is non-nil, the peer’s result may still be unresolved.
- `Step` evaluates a `kont.Expr[A]` to a result or a suspension. Give the suspension one owner to try the pending operation with `Advance` or abandon it with `Discard`. On `iox.ErrWouldBlock`, retry the same suspension after the peer makes progress. Success consumes the old suspension. If the next suspension is nil, the result is final.
- A successful send enqueues the payload. If the sender needs evidence of peer processing, have the peer acknowledge after processing.
- `SelectL` and `SelectR` send branch labels to `Offer`. Payloads use a separate queue. A `RecvBind` or `ExprRecvBind` callback can branch on the value it receives. Across endpoint pairs, ensure the peer can reach the `Recv` or `Offer` that frees queue space.
- When delegating a `*sess.Endpoint`, the sender retains ownership on `iox.ErrWouldBlock`. Successful enqueue puts the endpoint in transit. The recipient takes ownership on dequeue, so arrange for the receiving role to run.
- Build a fresh role for each run when using fused Expr constructors such as `ExprSendThen`. Each evaluation consumes its frames.
- Call `Advance` directly when only queue availability matters. Use `tempo` for decisions based on event history and `cove` for requirements on the current context. Recheck those conditions before each retry.

Run `make check` and `make test` with Go 1.27+. Run `make bench` with the Atomix compiler for transport measurements. [Benchmark guide](BENCHMARKS.md)

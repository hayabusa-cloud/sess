# Measuring sess

Run the suite on Linux with the Go 1.27.x Atomix compiler on `PATH`. Record the compiler version, CPU, kernel, dependency versions and `GOMAXPROCS` when comparing revisions. [Compiler releases](https://github.com/hayabusa-cloud/go/releases)

```sh
GOWORK=off GOTOOLCHAIN=local GOMAXPROCS=2 make bench
```

The paired workloads build the programs and create their endpoints inside each iteration. Compare the Cont and Expr variants for send/receive, a three-step exchange, branch selection and recursion. `BenchmarkExprSendRecvStructResult` returns a 32-byte summary from the sender. `BenchmarkExprRecvBindStructResult` builds one in the receive callback and returns it after `Close`. Delegation, `Exec` and concurrent stepping also include peer goroutine setup and handoff.

`BenchmarkExprRecvLoopStructResult` receives the batch size, reads that many integer messages in a loop, and returns a 32-byte summary.

`BenchmarkSendRetry` measures one refused `Advance` on a full queue. Each iteration retries the same pending pointer send. `BenchmarkSendRetryBytes` and `BenchmarkSendRetryValue32` repeat it with a nonempty byte slice and a 32-byte value. `BenchmarkSendRetryBytesPreboxed` constructs `Send[any]` before timing so retries reuse the boxed slice. `BenchmarkChannelSendRetry` is the corresponding pointer send on a full buffered Go channel.

`BenchmarkSendDelivery` steps fresh suspensions on an existing endpoint pair. Its pointer, byte-slice and 32-byte cases reuse their expressions. The `FreshBytes/Typed` and `FreshBytes/Preboxed` cases also time constructing each send from a concrete slice. Both deliver to `Recv[[]byte]`.

`BenchmarkRecvRetry` retries the same pending receive on an empty queue. The two ready cases time one `Advance` with an integer already queued and check its return. `BenchmarkRecvReady` returns the integer. `BenchmarkRecvReadyReply` uses it to construct a reply and yields the pending `Send[int]`. Both prepare up to 256 endpoint pairs on one goroutine before timing. The reply case sends the reply and verifies its value at the peer after the timer stops.

`BenchmarkStepAdvance` and `BenchmarkStepAdvancePeerWake` time the whole one-way exchange, including the goroutine and channel. The first polls after a refused receive. The second waits for the sender to finish before retrying. `refusals/op` counts empty receives per exchange. In a request/reply exchange, the sender can signal after enqueueing the request and before waiting for its reply.

To compare revisions, use the same benchmark source on the same machine and compare all ten samples with `benchstat`. Earlier versions of `BenchmarkRecLoop` added a `kont.Bind` after each send and the final close.

```sh
GOWORK=off GOTOOLCHAIN=local GOMAXPROCS=2 go test -mod=readonly -run '^$' \
  -bench '^Benchmark((Expr)?(Send|Recv|RecLoop)|ChannelSend|StepAdvance)' \
  -benchmem -benchtime=200ms -count=10 -timeout=5m .
```

Each GitHub Actions run shows a sample for every workload in its job summary. Download the `sess-benchmarks` artifact for the full measurements and runner environment. [Benchmark runs](https://github.com/hayabusa-cloud/sess/actions/workflows/benchmarks.yml)

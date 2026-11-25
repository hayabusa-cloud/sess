# Measuring sess

Run the suite on Linux with the Go 1.27.x Atomix compiler on `PATH`. Record the compiler version, CPU, kernel, dependency versions and `GOMAXPROCS` when comparing revisions. [Compiler releases](https://github.com/hayabusa-cloud/go/releases)

```sh
GOWORK=off GOTOOLCHAIN=local GOMAXPROCS=2 make bench
```

The paired workloads build the programs and create their endpoints inside each iteration. Compare the Cont and Expr variants for send/receive, a three-step exchange, branch selection and recursion. `BenchmarkExprSendRecvStructResult` returns a 32-byte summary from the sending role. Delegation, `Exec` and concurrent stepping also include peer goroutine setup and handoff.

`BenchmarkSendRetry` measures one `Advance` on an already-full queue, reusing the pending send. `BenchmarkChannelSendRetry` measures the corresponding refused send on a full buffered Go channel. Both use a pointer payload and exclude setup. Only the session workload carries a continuation.

To compare two revisions, run the same workloads on the same machine and keep all ten samples. For example, run the following command in each checkout, then compare the saved outputs with `benchstat`.

```sh
GOWORK=off GOTOOLCHAIN=local GOMAXPROCS=2 go test -mod=readonly -run '^$' \
  -bench '^Benchmark(SendRetry|ChannelSendRetry|ExprSendRecv|SendRecv)$' \
  -benchmem -benchtime=200ms -count=10 -timeout=5m .
```

Each GitHub Actions run shows a sample for every workload in its job summary. Download the `sess-benchmarks` artifact for the full measurements and runner environment. [Benchmark runs](https://github.com/hayabusa-cloud/sess/actions/workflows/benchmarks.yml)

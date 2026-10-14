# Application lifecycle

`Run` owns startup, shutdown, and dependency cleanup. `setup` registers HTTP
handlers, gRPC services, health probes, background worker completion channels,
and dependency cleanup functions before either server starts listening.

1. Load configuration and initialize logging, health checks, metrics, and tracing.
2. Run `setup` to initialize dependencies and register application components.
   Background workers may already start here.
3. Start HTTP and, when configured, gRPC servers concurrently.
4. On SIGINT, SIGTERM, or a server error, cancel `drainingCtx`: HTTP readiness
   fails and workers stop taking new work. Mark gRPC health as not serving.
   Keep the work context alive so active operations can finish.
5. Wait 10 seconds for load balancers to remove the instance, then close
   `stopAccepting` so both servers begin graceful shutdown.
6. Allow servers and workers a shared 30-second grace period. Wait for servers,
   then worker completion; they continue running concurrently during these waits.
   If the period expires, cancel `workCtx` to force server shutdown and release
   the worker wait. If everything finishes earlier, stop the timer.
7. Cancel the work context and clean up dependencies in reverse registration
   order, after server connections have closed. Finally, deferred telemetry and
   logging cleanup runs.

Configuration and setup errors are fatal startup failures; they do not enter
the graceful shutdown sequence.

## Coordination responsibilities

| Context or channel | Purpose | Why it remains |
| --- | --- | --- |
| `workCtx` | Lifetime of active operations | Canceling it at the start of draining would interrupt requests and messages before their grace period. |
| `drainingCtx` | Readiness and stopping new worker tasks | This happens 10 seconds before servers stop accepting requests and up to 40 seconds before forced cancellation. |
| `stopAccepting` | Begin graceful shutdown of both servers | This is a separate phase from draining and canceling active work. A closed channel broadcasts it without another context. |
| Each registered worker's `done` | Report actual worker completion | Requesting cancellation does not mean a worker has finished using its dependencies. |

`servers` is a wait group because server completion only needs a blocking wait.
No aggregate `workDone` channel or goroutine is needed: the grace timer cancels
work independently while `Run` waits directly. Its `time.AfterFunc` callback
also avoids selecting on a timer channel.

Worker waits stop on forced cancellation even if a worker never closes its
completion channel. Workers must respect the work context to finish safely;
cancellation cannot forcibly terminate a Go goroutine.

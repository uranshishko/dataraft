# DataRaft – Type-safe, concurrent ETL for Go

`dataraft` is a **generic, type-safe, concurrent ETL (Extract, Transform, Load) library** for Go. It supports **typed records with IDs**, concurrent pipelines, retries with backoff, execution history, and scheduling for recurring jobs.

---

## Installation

```bash
go get github.com/uranshishko/dataraft
```

---

## Key Concepts

- **TypedJob[TIn, TOut]** – a generic job that extracts, transforms, and loads data.
- **Record[T] / TransformedRecord[T]** – wrapper types for source data and transformed results, each with a unique ID.
- **PipelineResult** – holds batch execution results and errors.
- **Orchestrator** – central manager for registering, executing, and tracking jobs.
- **Scheduler** – schedules recurring jobs at specified intervals.
- **RetryConfig** – configure retries with exponential backoff.
- **ExecutionResult** – tracks detailed execution info (status, attempts, duration, errors, metadata).

---

## Importing

```go
import "github.com/uranshishko/dataraft"
```

---

## Basic ETL Example

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/uranshishko/dataraft"
)

// Dummy source returning numbers
type intSource struct{}
func (s *intSource) Fetch(ctx context.Context) ([]int, error) { return []int{1, 2, 3}, nil }

// Dummy loader printing results for demonstration
type intLoader struct{}
func (l *intLoader) Load(ctx context.Context, records []int) error {
	fmt.Println("Loaded:", records)
	return nil
}

func main() {
	source := &intSource{}
	loader := &intLoader{}

	transform := func(r dataraft.Record[int]) (dataraft.TransformedRecord[int], error) {
		return dataraft.TransformedRecord[int]{ID: r.ID, Result: r.Data * 2}, nil
	}

	job := dataraft.NewTypedJob(
		"double",
		"Double numbers",
		source,
		transform,
		loader,
		2,
		10,
	)

	orch := dataraft.NewOrchestrator(nil)
	_ = orch.RegisterJob(job)

	result, _ := orch.ExecuteJob(context.Background(), "double", nil)
	fmt.Println("Execution status:", result.GetStatus())
}
```

---

## Scheduler Example

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/uranshishko/dataraft"
)

// Simple job that prints a message
type printJob struct{ message string }

func (j *printJob) Name() string        { return "printJob" }
func (j *printJob) Description() string { return "Prints a message" }
func (j *printJob) Execute(ctx context.Context) (*dataraft.PipelineResult, error) {
	fmt.Println(j.message)
	return &dataraft.PipelineResult{}, nil
}

func main() {
	orch := dataraft.NewOrchestrator(nil)
	job := &printJob{message: "Hello from scheduled job!"}

	_ = orch.RegisterJob(job)
	_ = orch.Scheduler().AddSchedule("printJob", 1*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	orch.Scheduler().Start(ctx)
}
```

> **Note:** Scheduled jobs may run multiple times depending on the ticker interval.

---

## Functional Options for Jobs

```go
job := dataraft.NewTypedJob(
	"example",
	"Example job",
	source,
	transform,
	loader,
	2,
	10,
	dataraft.WithTimeout[int, int](time.Minute),
	dataraft.WithRetry[int, int](&dataraft.RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  time.Second,
		MaxDelay:      5 * time.Second,
		BackoffFactor: 2.0,
	}),
)
```

- `WithTimeout` sets a maximum job duration.
- `WithRetry` configures retries and backoff.

> Functional options provide a **cleaner, immutable-style API** compared to setters.

---

## Execution History

```go
orch := dataraft.NewOrchestrator(nil)
executions := orch.GetRecentExecutions(10) // most recent 10
history := orch.GetJobHistory("double", 5) // last 5 executions
```

Each `ExecutionResult` provides:

- `Status` (Pending, Running, Completed, Failed, Cancelled, Retrying)
- `StartTime`, `EndTime`, `Duration`
- `Attempt` count
- `PipelineResult` and `Error`
- Optional metadata

---

## Cancel Running Jobs

```go
result, _ := orch.ExecuteJob(ctx, "double", nil)
err := orch.CancelExecution(result.ExecutionID)
```

Cancels an active execution and updates its status to `CANCELLED`.

---

## Custom Retry Strategy

```go
retry := &dataraft.RetryConfig{
	MaxAttempts:   3,
	InitialDelay:  1 * time.Second,
	MaxDelay:      10 * time.Second,
	BackoffFactor: 2,
}

job := dataraft.NewTypedJob(
	"exampleRetry",
	"Job with retries",
	source,
	transform,
	loader,
	2,
	10,
	dataraft.WithRetry(retry),
)
```

---

## Notes on Concurrency

- `DataRaft` pipelines and schedulers are **concurrent**.
- When running tests with `--race`, job execution order may vary, and scheduled jobs may print multiple times.
- Execution history and results are **thread-safe**.

---

## License

MIT License © 2026 Uran Shishko

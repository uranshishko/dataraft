<p align="center">
  <img src="/images/gopher_dataraft.png" alt="Gopher with a DataRaft"/>
</p>

# DataRaft – Type-safe, concurrent ETL for Go

`dataraft` is a **generic, type-safe, concurrent ETL (Extract, Transform, Load) library** for Go. It provides a composable pipeline with typed records, concurrency, retries, execution tracking, and scheduling.

---

## Installation

```bash
go get github.com/uranshishko/dataraft
```

---

## Key Concepts

- **TypedJob[TIn, TOut]** – defines a full ETL job
- **Record[T]** – input record with ID and data
- **TransformedRecord[T]** – output record after transformation
- **DataSource[T]** – produces records via channel
- **DataLoader[T]** – consumes transformed records via channel
- **PipelineResult** – tracks processed/failed records and errors
- **Orchestrator** – runs and manages jobs
- **Scheduler** – runs jobs on intervals
- **RetryConfig** – retry strategy with backoff

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

	"github.com/uranshishko/dataraft"
)

type intSource struct{}

func (s *intSource) Extract(ctx context.Context, out chan<- dataraft.Record[int]) error {
	for i := 1; i <= 5; i++ {
		out <- dataraft.Record[int]{ID: int64(i), Data: i}
	}
	return nil
}

type intLoader struct{}

func (l *intLoader) Load(ctx context.Context, in <-chan dataraft.TransformedRecord[int]) error {
	for r := range in {
		fmt.Println("Loaded:", r.Result)
	}
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
	fmt.Println("Status:", result.GetStatus())
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

type printJob struct{}

func (j *printJob) Name() string        { return "print" }
func (j *printJob) Description() string { return "prints a message" }

func (j *printJob) Execute(ctx context.Context) (*dataraft.PipelineResult, error) {
	fmt.Println("Hello from scheduler")
	return &dataraft.PipelineResult{}, nil
}

func main() {
	orch := dataraft.NewOrchestrator(nil)

	_ = orch.RegisterJob(&printJob{})
	_ = orch.Scheduler().AddSchedule("print", 1*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	orch.Scheduler().Start(ctx)
}
```

---

## Homebrew Monitoring Example

While `dataraft` does not include a built-in UI, you can easily build lightweight monitoring using the orchestrator APIs.

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/uranshishko/dataraft"
)

func main() {
	orch := dataraft.NewOrchestrator(nil)

	// Register jobs (example assumes jobs already exist)
	_ = orch.RegisterJob(job1)
	_ = orch.RegisterJob(job2)

	// Setup scheduler
	scheduler := orch.Scheduler()
	_ = scheduler.AddSchedule("job-1", 1*time.Hour)
	_ = scheduler.AddSchedule("job-2", 6*time.Hour)

	// Start scheduler
	ctx := context.Background()
	go scheduler.Start(ctx)

	// Print registered jobs
	fmt.Println("\nRegistered Jobs:")
	for _, info := range orch.ListJobs() {
		fmt.Printf("  - %s: %s\n", info.Name, info.Description)
	}

	// Simple monitoring loop
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			active := orch.GetActiveExecutions()
			recent := orch.GetRecentExecutions(5)

			fmt.Println("\n📊 Status Report:")

			// Active executions
			fmt.Printf("Active executions: %d\n", len(active))
			for _, id := range active {
				fmt.Printf("  - %s\n", id)
			}

			// Recent executions
			fmt.Println("Recent executions:")
			for _, exec := range recent {
				fmt.Printf(
					"  - %s [%s] (%s, attempt %d)\n",
					exec.JobName,
					exec.Status,
					exec.Duration,
					exec.Attempt,
				)
			}
		}
	}()

	// Keep app running
	select {}
}
```

## Prometheus metrics monitoring

You can also use the `dataraft` orchestrator to export Prometheus metrics for monitoring and alerting. This can be useful for monitoring the health and performance of your ETL pipelines.

```go
package main

import (
	"context"
	"net/http"
	"time"

	"github.com/uranshishko/dataraft"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	orch := dataraft.NewOrchestrator(nil)

	// Example jobs (assume job1 and job2 are already defined)
	_ = orch.RegisterJob(job1)
	_ = orch.RegisterJob(job2)

	// Scheduler setup
	scheduler := orch.Scheduler()
	_ = scheduler.AddSchedule("job-1", 1*time.Hour)
	_ = scheduler.AddSchedule("job-2", 6*time.Hour)
	go scheduler.Start(context.Background())

	// Prometheus metrics
	activeJobs := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dataraft_active_jobs",
		Help: "Number of active job executions",
	})
	recentExecutions := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dataraft_job_executions_total",
			Help: "Total number of job executions by status",
		},
		[]string{"job", "status"},
	)

	prometheus.MustRegister(activeJobs, recentExecutions)

	// Monitoring loop
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			active := orch.GetActiveExecutions()
			activeJobs.Set(float64(len(active)))

			for _, exec := range orch.GetRecentExecutions(10) {
				recentExecutions.WithLabelValues(exec.JobName, string(exec.Status)).Inc()
			}
		}
	}()

	// Expose Prometheus endpoint
	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(":2112", nil)
}
```

---

## Job Configuration

```go
job := dataraft.NewTypedJob(
	"name",
	"description",
	source,
	transform,
	loader,
	workers,
	batchSize,
).
	WithTimeout(2 * time.Minute).
	WithRetry(&dataraft.RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  time.Second,
		MaxDelay:      5 * time.Second,
		BackoffFactor: 2,
	})
```

---

## Execution Tracking

```go
result, _ := orch.ExecuteJob(ctx, "double", nil)

fmt.Println(result.Status)
fmt.Println(result.Duration)
fmt.Println(result.Attempt)
```

---

## Execution History

```go
recent := orch.GetRecentExecutions(10)
history := orch.GetJobHistory("double", 5)
```

---

## Cancel Execution

```go
res, _ := orch.ExecuteJob(ctx, "double", nil)
_ = orch.CancelExecution(res.ExecutionID)
```

---

## Retry Configuration

```go
retry := &dataraft.RetryConfig{
	MaxAttempts:   3,
	InitialDelay:  time.Second,
	MaxDelay:      10 * time.Second,
	BackoffFactor: 2,
}
```

---

## Concurrency Notes

- Pipelines run with multiple workers
- Ordering is **not guaranteed**
- Scheduler timing is approximate (ticker-based)
- Safe for concurrent use

---

## License

MIT License © 2026 Uran Shishko

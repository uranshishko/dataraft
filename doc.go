// Package dataraft provides a type-safe, generic, concurrent ETL (Extract, Transform, Load)
// framework for Go. It allows you to define typed jobs that fetch data from a source,
// transform it, and load it into a destination while supporting concurrency, retries,
// execution tracking, and scheduling.
//
// Key Concepts:
//
//   - TypedJob[TIn, TOut]: A generic job with typed input and output. Each job can
//     fetch, transform, and load data concurrently.
//   - Record[T] / TransformedRecord[T]: Wrappers for source and transformed data,
//     each with a unique ID for tracking.
//   - PipelineResult: Captures the outcome of a batch pipeline, including errors.
//   - Orchestrator: Central component for registering, executing, and monitoring jobs.
//   - Scheduler: Schedules recurring jobs at fixed intervals.
//   - RetryConfig: Configures retries with exponential backoff for failed jobs.
//   - ExecutionResult: Provides detailed information about job executions, including
//     status, start/end time, duration, attempt count, errors, and metadata.
//
// Example: Simple ETL job
//
//	package main
//
//	import (
//	    "context"
//	    "fmt"
//	    "time"
//
//	    "github.com/uranshishko/dataraft"
//	)
//
//	type intSource struct{}
//	func (s *intSource) Fetch(ctx context.Context) ([]int, error) { return []int{1, 2, 3}, nil }
//
//	type intLoader struct{}
//	func (l *intLoader) Load(ctx context.Context, records []int) error {
//	    fmt.Println("Loaded:", records)
//	    return nil
//	}
//
//	func main() {
//	    source := &intSource{}
//	    loader := &intLoader{}
//
//	    transform := func(r dataraft.Record[int]) (dataraft.TransformedRecord[int], error) {
//	        return dataraft.TransformedRecord[int]{ID: r.ID, Result: r.Data * 2}, nil
//	    }
//
//	    job := dataraft.NewTypedJob(
//	        "double",
//	        "Double numbers",
//	        source,
//	        transform,
//	        loader,
//	        2,
//	        10,
//	    )
//
//	    orch := dataraft.NewOrchestrator(nil)
//	    _ = orch.RegisterJob(job)
//
//	    result, _ := orch.ExecuteJob(context.Background(), "double", nil)
//	    fmt.Println("Execution status:", result.GetStatus())
//	}
//
// Package dataraft is safe for concurrent usage, and is designed to be
// extensible and type-safe, making it suitable for robust ETL pipelines in Go.
package dataraft

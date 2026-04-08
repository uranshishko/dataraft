// Package dataraft provides a type-safe, generic, concurrent ETL (Extract, Transform, Load)
// framework for Go. It allows you to define typed jobs that extract data from a source,
// transform it, and load it into a destination while supporting concurrency, retries,
// execution tracking, and scheduling.
//
// Key Concepts:
//
//   - TypedJob[TIn, TOut]: A generic job with typed input and output.
//   - Record[T] / TransformedRecord[T]: Wrappers for source and transformed data,
//     each with a unique ID for tracking.
//   - PipelineResult: Captures the outcome of a pipeline execution, including errors.
//   - Orchestrator: Central component for registering, executing, and monitoring jobs.
//   - Scheduler: Schedules recurring jobs at fixed intervals.
//   - RetryConfig: Configures retries with exponential backoff.
//   - ExecutionResult: Provides detailed information about job executions.
//
// Note:
//
//	Pipelines are executed concurrently. Record ordering is not guaranteed.
//
// Example:
//
//	package main
//
//	import (
//	    "context"
//	    "fmt"
//
//	    "github.com/uranshishko/dataraft"
//	)
//
//	type intSource struct{}
//
//	func (s *intSource) Extract(ctx context.Context, out chan<- dataraft.Record[int]) error {
//	    for i := 1; i <= 5; i++ {
//	        out <- dataraft.Record[int]{ID: int64(i), Data: i}
//	    }
//	    return nil
//	}
//
//	type intLoader struct{}
//
//	func (l *intLoader) Load(ctx context.Context, in <-chan dataraft.TransformedRecord[int]) error {
//	    for r := range in {
//	        fmt.Println(r.Result)
//	    }
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
// Package dataraft is safe for concurrent usage and is designed to be
// extensible and type-safe, making it suitable for robust ETL pipelines in Go.
package dataraft

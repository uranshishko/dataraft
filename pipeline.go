package dataraft

import (
	"context"
	"fmt"
	"sync"
)

// Record represents a single input item in the pipeline.
type Record[T any] struct {
	ID   int64
	Data T
}

// TransformedRecord represents the output of a transformation stage.
type TransformedRecord[T any] struct {
	ID          int64
	ProcessedAt int64
	Result      T
}

// DataSource defines a source that produces records into the pipeline.
type DataSource[T any] interface {
	Extract(context.Context, chan<- Record[T]) error
}

// DataLoader defines a sink that consumes transformed records.
type DataLoader[T any] interface {
	Load(context.Context, <-chan TransformedRecord[T]) error
}

// TransformFn defines a transformation from input record to output record.
type TransformFn[TIn, TOut any] func(Record[TIn]) (TransformedRecord[TOut], error)

// PipelineResult contains execution metrics and errors.
type PipelineResult struct {
	Processed int64
	Failed    int64
	Errors    []error
	mu        sync.Mutex
}

// RecordError registers a failed record and stores the error (up to 100).
func (r *PipelineResult) RecordError(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Failed++
	if len(r.Errors) < 100 {
		r.Errors = append(r.Errors, err)
	}
}

// RecordSuccess increments the processed record count.
func (r *PipelineResult) RecordSuccess() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Processed++
}

// ShouldAbort determines whether the pipeline should stop based on failure rate.
// Evaluation starts after at least 100 processed records.
func (r *PipelineResult) ShouldAbort(threshold float64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	total := r.Processed + r.Failed
	if total < 100 {
		return false
	}

	failRate := float64(r.Failed) / float64(total)
	return failRate > threshold
}

// Pipeline coordinates extraction, transformation, and loading.
type Pipeline[TIn, TOut any] struct {
	workerCount int
	batchSize   int
}

// NewPipeline creates a new Pipeline with sane defaults.
func NewPipeline[TIn, TOut any](workers, batchSize int) *Pipeline[TIn, TOut] {
	if workers <= 0 {
		workers = 1
	}
	if batchSize <= 0 {
		batchSize = 100
	}

	return &Pipeline[TIn, TOut]{
		workerCount: workers,
		batchSize:   batchSize,
	}
}

// runWorkers processes records concurrently using the configured worker pool.
func (p *Pipeline[TIn, TOut]) runWorkers(
	ctx context.Context,
	input <-chan Record[TIn],
	output chan<- TransformedRecord[TOut],
	transformFn TransformFn[TIn, TOut],
	result *PipelineResult,
) {
	var wg sync.WaitGroup

	for i := 0; i < p.workerCount; i++ {
		wg.Add(1)

		go func(workerID int) {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				case record, ok := <-input:
					if !ok {
						return
					}

					if result.ShouldAbort(0.1) {
						return
					}

					out, err := transformFn(record)
					if err != nil {
						result.RecordError(
							fmt.Errorf("worker %d: transform failed for record %d: %w",
								workerID, record.ID, err),
						)
						continue
					}

					select {
					case output <- out:
						result.RecordSuccess()
					case <-ctx.Done():
						return
					}
				}
			}
		}(i)
	}

	wg.Wait()
	close(output)
}

// Run executes the full ETL pipeline.
func (p *Pipeline[TIn, TOut]) Run(
	ctx context.Context,
	source DataSource[TIn],
	transformFn TransformFn[TIn, TOut],
	loader DataLoader[TOut],
) *PipelineResult {
	extracted := make(chan Record[TIn], p.batchSize)
	transformed := make(chan TransformedRecord[TOut], p.batchSize)

	var extractErr, loadErr error
	var wg sync.WaitGroup
	result := &PipelineResult{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(extracted)
		extractErr = source.Extract(ctx, extracted)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		p.runWorkers(ctx, extracted, transformed, transformFn, result)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		loadErr = loader.Load(ctx, transformed)
	}()

	wg.Wait()

	if extractErr != nil {
		result.RecordError(fmt.Errorf("extract failed: %w", extractErr))
	}
	if loadErr != nil {
		result.RecordError(fmt.Errorf("load failed: %w", loadErr))
	}

	return result
}

// BatchLoader consumes transformed records in batches and calls loadBatch for each batch.
func BatchLoader[T any](
	ctx context.Context,
	input <-chan TransformedRecord[T],
	batchSize int,
	loadBatch func([]TransformedRecord[T]) error,
) error {
	batch := make([]TransformedRecord[T], 0, batchSize)

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := loadBatch(batch)
		batch = batch[:0]
		return err
	}

	for {
		select {
		case <-ctx.Done():
			if err := flush(); err != nil {
				return fmt.Errorf("flush on context cancel: %w", err)
			}
			return ctx.Err()

		case record, ok := <-input:
			if !ok {
				return flush()
			}

			batch = append(batch, record)
			if len(batch) >= batchSize {
				if err := flush(); err != nil {
					return err
				}
			}
		}
	}
}

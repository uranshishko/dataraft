package dataraft

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Job is the base interface that all jobs must implement.
// A Job represents a unit of work that can be executed by the orchestrator.
type Job interface {
	Name() string
	Description() string
	Execute(ctx context.Context) (*PipelineResult, error)
}

// TypedJob wraps a pipeline execution with specific input and output types.
type TypedJob[TIn, TOut any] struct {
	name        string
	description string
	source      DataSource[TIn]
	transform   TransformFn[TIn, TOut]
	loader      DataLoader[TOut]
	workers     int
	batchSize   int
	timeout     time.Duration
	retryConfig *RetryConfig
}

// JobOption is a function that can be used to configure a TypedJob.
type JobOption[TIn, TOut any] func(*TypedJob[TIn, TOut])

// WithTimeout sets a maximum execution duration for the job.
func WithTimeout[TIn, TOut any](timeout time.Duration) JobOption[TIn, TOut] {
	return func(j *TypedJob[TIn, TOut]) {
		j.timeout = timeout
	}
}

// WithRetry configures retry behavior for the job.
func WithRetry[TIn, TOut any](config *RetryConfig) JobOption[TIn, TOut] {
	return func(j *TypedJob[TIn, TOut]) {
		j.retryConfig = config
	}
}

// NewTypedJob creates a new TypedJob with the provided configuration.
// Defaults are applied for workers and batch size if invalid values are provided.
func NewTypedJob[TIn, TOut any](
	name, description string,
	source DataSource[TIn],
	transform TransformFn[TIn, TOut],
	loader DataLoader[TOut],
	workers, batchSize int,
	opts ...JobOption[TIn, TOut],
) *TypedJob[TIn, TOut] {
	if workers <= 0 {
		workers = 4
	}
	if batchSize <= 0 {
		batchSize = 100
	}

	job := &TypedJob[TIn, TOut]{
		name:        name,
		description: description,
		source:      source,
		transform:   transform,
		loader:      loader,
		workers:     workers,
		batchSize:   batchSize,
		timeout:     5 * time.Minute,
	}

	for _, opt := range opts {
		opt(job)
	}

	return job
}

// Name returns the job name.
func (j *TypedJob[TIn, TOut]) Name() string {
	return j.name
}

// Description returns a human-readable description of the job.
func (j *TypedJob[TIn, TOut]) Description() string {
	return j.description
}

// Execute runs the underlying ETL pipeline and returns the result.
// If a timeout is configured, the context is wrapped accordingly.
func (j *TypedJob[TIn, TOut]) Execute(ctx context.Context) (*PipelineResult, error) {
	// Apply timeout if configured
	if j.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, j.timeout)
		defer cancel()
	}

	pipeline := NewPipeline[TIn, TOut](j.workers, j.batchSize)
	result := pipeline.Run(ctx, j.source, j.transform, j.loader)

	if len(result.Errors) > 0 {
		return result, fmt.Errorf("pipeline completed with %d errors", len(result.Errors))
	}

	return result, nil
}

// RetryConfig defines retry behavior for job execution.
type RetryConfig struct {
	MaxAttempts   int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	BackoffFactor float64
}

// JobRegistry manages registration and lookup of jobs.
type JobRegistry struct {
	jobs map[string]Job
	mu   sync.RWMutex
}

// NewJobRegistry creates a new JobRegistry instance.
func NewJobRegistry() *JobRegistry {
	return &JobRegistry{
		jobs: make(map[string]Job),
	}
}

// Register adds a job to the registry.
// Returns an error if the job name is empty or already exists.
func (r *JobRegistry) Register(job Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if job.Name() == "" {
		return fmt.Errorf("job name cannot be empty")
	}

	if _, exists := r.jobs[job.Name()]; exists {
		return fmt.Errorf("job %s already registered", job.Name())
	}

	r.jobs[job.Name()] = job
	return nil
}

// Get retrieves a job by name.
func (r *JobRegistry) Get(name string) (Job, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	job, ok := r.jobs[name]
	return job, ok
}

// List returns metadata for all registered jobs.
func (r *JobRegistry) List() []JobInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info := make([]JobInfo, 0, len(r.jobs))
	for _, job := range r.jobs {
		info = append(info, JobInfo{
			Name:        job.Name(),
			Description: job.Description(),
		})
	}
	return info
}

// Unregister removes a job from the registry.
// Returns true if the job existed and was removed.
func (r *JobRegistry) Unregister(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.jobs[name]; !exists {
		return false
	}

	delete(r.jobs, name)
	return true
}

// JobInfo contains basic metadata about a job.
type JobInfo struct {
	Name        string
	Description string
}

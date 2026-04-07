package dataraft

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ExecutionStatus represents the lifecycle state of a job execution.
type ExecutionStatus string

const (
	StatusPending   ExecutionStatus = "PENDING"
	StatusRunning   ExecutionStatus = "RUNNING"
	StatusCompleted ExecutionStatus = "COMPLETED"
	StatusFailed    ExecutionStatus = "FAILED"
	StatusCancelled ExecutionStatus = "CANCELLED"
	StatusRetrying  ExecutionStatus = "RETRYING"
)

// ExecutionResult holds the result and metadata of a job execution.
type ExecutionResult struct {
	ExecutionID string
	JobName     string
	Status      ExecutionStatus
	StartTime   time.Time
	EndTime     time.Time
	Duration    time.Duration
	Attempt     int
	Pipeline    *PipelineResult
	Error       error
	Metadata    map[string]any
	mu          sync.RWMutex
}

// SetStatus updates the execution status.
func (r *ExecutionResult) SetStatus(status ExecutionStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Status = status
}

// GetStatus returns the current execution status.
func (r *ExecutionResult) GetStatus() ExecutionStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Status
}

// Complete finalizes the execution result by setting timing, status, and error state.
func (r *ExecutionResult) Complete(pipelineResult *PipelineResult, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.EndTime = time.Now()
	r.Duration = r.EndTime.Sub(r.StartTime)
	r.Pipeline = pipelineResult

	if err != nil || (pipelineResult != nil && len(pipelineResult.Errors) > 0) {
		r.Status = StatusFailed
		r.Error = err
	} else {
		r.Status = StatusCompleted
	}
}

// ExecutionHistory stores execution results with a bounded size.
type ExecutionHistory struct {
	executions map[string]*ExecutionResult
	ordered    []*ExecutionResult // For efficient time-based queries
	mu         sync.RWMutex
	maxSize    int
}

// NewExecutionHistory creates a new ExecutionHistory with a maximum size.
func NewExecutionHistory(maxSize int) *ExecutionHistory {
	return &ExecutionHistory{
		executions: make(map[string]*ExecutionResult),
		ordered:    make([]*ExecutionResult, 0),
		maxSize:    maxSize,
	}
}

// Add stores a new execution result and evicts the oldest if the limit is exceeded.
func (h *ExecutionHistory) Add(result *ExecutionResult) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.executions[result.ExecutionID] = result
	h.ordered = append(h.ordered, result)

	// Remove oldest if exceeding max size
	if len(h.ordered) > h.maxSize {
		oldest := h.ordered[0]
		delete(h.executions, oldest.ExecutionID)
		h.ordered = h.ordered[1:]
	}
}

// Get retrieves an execution result by ID.
func (h *ExecutionHistory) Get(executionID string) (*ExecutionResult, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result, ok := h.executions[executionID]
	return result, ok
}

// GetByJob returns execution results for a specific job, ordered by most recent first.
func (h *ExecutionHistory) GetByJob(jobName string, limit int) []*ExecutionResult {
	h.mu.RLock()
	defer h.mu.RUnlock()

	results := make([]*ExecutionResult, 0)

	// Iterate in reverse order (most recent first)
	for i := len(h.ordered) - 1; i >= 0; i-- {
		if h.ordered[i].JobName == jobName {
			results = append(results, h.ordered[i])
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}

	return results
}

// GetRecent returns the most recent execution results.
func (h *ExecutionHistory) GetRecent(limit int) []*ExecutionResult {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if limit <= 0 || limit > len(h.ordered) {
		limit = len(h.ordered)
	}

	results := make([]*ExecutionResult, limit)
	for i := 0; i < limit; i++ {
		results[i] = h.ordered[len(h.ordered)-1-i]
	}

	return results
}

// Orchestrator coordinates job execution, scheduling, and history tracking.
type Orchestrator struct {
	registry  *JobRegistry
	history   *ExecutionHistory
	scheduler *Scheduler

	// Active executions tracking
	activeExecutions map[string]context.CancelFunc
	activeMu         sync.RWMutex

	// Hooks for monitoring
	onExecutionStart func(*ExecutionResult)
	onExecutionEnd   func(*ExecutionResult)
}

// OrchestratorConfig defines configuration for the Orchestrator.
type OrchestratorConfig struct {
	MaxHistorySize   int
	OnExecutionStart func(*ExecutionResult)
	OnExecutionEnd   func(*ExecutionResult)
}

// NewOrchestrator creates a new Orchestrator instance.
func NewOrchestrator(config *OrchestratorConfig) *Orchestrator {
	if config == nil {
		config = &OrchestratorConfig{
			MaxHistorySize: 1000,
		}
	}

	orch := &Orchestrator{
		registry:         NewJobRegistry(),
		history:          NewExecutionHistory(config.MaxHistorySize),
		activeExecutions: make(map[string]context.CancelFunc),
		onExecutionStart: config.OnExecutionStart,
		onExecutionEnd:   config.OnExecutionEnd,
	}

	orch.scheduler = NewScheduler(orch)
	return orch
}

// RegisterJob registers a job with the orchestrator.
func (o *Orchestrator) RegisterJob(job Job) error {
	return o.registry.Register(job)
}

// ExecuteJob executes a job by name and tracks its execution lifecycle.
func (o *Orchestrator) ExecuteJob(ctx context.Context, jobName string, metadata map[string]any) (*ExecutionResult, error) {
	job, ok := o.registry.Get(jobName)
	if !ok {
		return nil, fmt.Errorf("job %s not found", jobName)
	}

	// Create execution result
	executionID := fmt.Sprintf("%s-%d", jobName, time.Now().UnixNano())
	result := &ExecutionResult{
		ExecutionID: executionID,
		JobName:     jobName,
		Status:      StatusPending,
		StartTime:   time.Now(),
		Attempt:     1,
		Metadata:    metadata,
	}

	o.history.Add(result)

	// Call start hook
	if o.onExecutionStart != nil {
		o.onExecutionStart(result)
	}

	// Execute the job
	err := o.executeJob(ctx, job, result)

	// Call end hook
	if o.onExecutionEnd != nil {
		o.onExecutionEnd(result)
	}

	return result, err
}

// executeJob handles execution, retries, and backoff logic for a job.
func (o *Orchestrator) executeJob(ctx context.Context, job Job, result *ExecutionResult) error {
	// Get retry config if job supports it
	var retryConfig *RetryConfig
	if typedJob, ok := job.(interface{ GetRetryConfig() *RetryConfig }); ok {
		retryConfig = typedJob.GetRetryConfig()
	}

	maxAttempts := 1
	if retryConfig != nil && retryConfig.MaxAttempts > 1 {
		maxAttempts = retryConfig.MaxAttempts
	}

	// Setup cancellable context
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Track active execution
	o.activeMu.Lock()
	o.activeExecutions[result.ExecutionID] = cancel
	o.activeMu.Unlock()

	defer func() {
		o.activeMu.Lock()
		delete(o.activeExecutions, result.ExecutionID)
		o.activeMu.Unlock()
	}()

	result.SetStatus(StatusRunning)

	var pipelineResult *PipelineResult
	var err error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result.Attempt = attempt

		if attempt > 1 {
			result.SetStatus(StatusRetrying)
			delay := o.calculateBackoff(attempt, retryConfig)

			select {
			case <-ctx.Done():
				result.Complete(pipelineResult, ctx.Err())
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		// Execute the job
		pipelineResult, err = job.Execute(ctx)

		// Success case
		if err == nil && (pipelineResult == nil || len(pipelineResult.Errors) == 0) {
			result.Complete(pipelineResult, nil)
			return nil
		}

		// Don't retry if context was cancelled
		if ctx.Err() != nil {
			result.Complete(pipelineResult, ctx.Err())
			return ctx.Err()
		}
	}

	// All retries exhausted
	result.Complete(pipelineResult, err)
	return err
}

// calculateBackoff computes the delay between retry attempts.
func (o *Orchestrator) calculateBackoff(attempt int, config *RetryConfig) time.Duration {
	if config == nil {
		return time.Second
	}

	delay := float64(config.InitialDelay)
	for i := 1; i < attempt; i++ {
		delay *= config.BackoffFactor
	}

	if time.Duration(delay) > config.MaxDelay {
		return config.MaxDelay
	}

	return time.Duration(delay)
}

// CancelExecution cancels an active execution by ID.
func (o *Orchestrator) CancelExecution(executionID string) error {
	o.activeMu.Lock()
	cancel, exists := o.activeExecutions[executionID]
	o.activeMu.Unlock()

	if !exists {
		return fmt.Errorf("execution %s not found or already completed", executionID)
	}

	cancel()

	// Update execution result
	if result, ok := o.history.Get(executionID); ok {
		result.SetStatus(StatusCancelled)
	}

	return nil
}

// GetExecutionStatus retrieves the status of an execution.
func (o *Orchestrator) GetExecutionStatus(executionID string) (*ExecutionResult, error) {
	result, ok := o.history.Get(executionID)
	if !ok {
		return nil, fmt.Errorf("execution %s not found", executionID)
	}
	return result, nil
}

// GetJobHistory returns execution history for a specific job.
func (o *Orchestrator) GetJobHistory(jobName string, limit int) []*ExecutionResult {
	return o.history.GetByJob(jobName, limit)
}

// GetRecentExecutions returns the most recent executions.
func (o *Orchestrator) GetRecentExecutions(limit int) []*ExecutionResult {
	return o.history.GetRecent(limit)
}

// ListJobs returns all registered jobs.
func (o *Orchestrator) ListJobs() []JobInfo {
	return o.registry.List()
}

// GetJob retrieves a job by name.
func (o *Orchestrator) GetJob(name string) (Job, bool) {
	return o.registry.Get(name)
}

// Scheduler returns the scheduler instance.
func (o *Orchestrator) Scheduler() *Scheduler {
	return o.scheduler
}

// GetActiveExecutions returns IDs of currently active executions.
func (o *Orchestrator) GetActiveExecutions() []string {
	o.activeMu.RLock()
	defer o.activeMu.RUnlock()

	executions := make([]string, 0, len(o.activeExecutions))
	for id := range o.activeExecutions {
		executions = append(executions, id)
	}
	return executions
}

// GetRetryConfig returns the retry configuration for the job.
func (j *TypedJob[TIn, TOut]) GetRetryConfig() *RetryConfig {
	return j.retryConfig
}

package dataraft_test

import (
	"context"
	"fmt"
	"time"

	"github.com/uranshishko/dataraft"
)

// Simple job that prints a message
type printJob struct {
	message string
}

func (j *printJob) Name() string        { return "printJob" }
func (j *printJob) Description() string { return "prints a message" }
func (j *printJob) Execute(ctx context.Context) (*dataraft.PipelineResult, error) {
	fmt.Println(j.message)
	return &dataraft.PipelineResult{}, nil
}

func ExampleScheduler() {
	orch := dataraft.NewOrchestrator(nil)
	job := &printJob{message: "Hello from scheduled job!"}

	_ = orch.RegisterJob(job)

	// Schedule the job every 1 second
	_ = orch.Scheduler().AddSchedule("printJob", 1*time.Second)

	// Run the scheduler for 3 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	orch.Scheduler().Start(ctx)

	// Output:
	// Hello from scheduled job!
	// Hello from scheduled job!
	// Hello from scheduled job!
}

package dataraft

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Schedule defines a recurring execution configuration for a job.
type Schedule struct {
	JobName  string
	Interval time.Duration
	Enabled  bool
	NextRun  time.Time
}

// Scheduler manages periodic execution of jobs.
type Scheduler struct {
	orchestrator *Orchestrator
	schedules    map[string]*Schedule
	mu           sync.RWMutex
	stopChan     chan struct{}
	running      bool
}

// NewScheduler creates a new Scheduler instance.
func NewScheduler(orchestrator *Orchestrator) *Scheduler {
	return &Scheduler{
		orchestrator: orchestrator,
		schedules:    make(map[string]*Schedule),
		stopChan:     make(chan struct{}),
	}
}

// AddSchedule registers a new schedule for a job.
func (s *Scheduler) AddSchedule(jobName string, interval time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Verify job exists
	if _, ok := s.orchestrator.registry.Get(jobName); !ok {
		return fmt.Errorf("job %s not found", jobName)
	}

	if _, exists := s.schedules[jobName]; exists {
		return fmt.Errorf("schedule for job %s already exists", jobName)
	}

	s.schedules[jobName] = &Schedule{
		JobName:  jobName,
		Interval: interval,
		Enabled:  true,
		NextRun:  time.Now().Add(interval),
	}

	return nil
}

// RemoveSchedule deletes a schedule for a job.
func (s *Scheduler) RemoveSchedule(jobName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.schedules[jobName]; !exists {
		return false
	}

	delete(s.schedules, jobName)
	return true
}

// EnableSchedule enables or disables a schedule.
func (s *Scheduler) EnableSchedule(jobName string, enabled bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	schedule, exists := s.schedules[jobName]
	if !exists {
		return false
	}

	schedule.Enabled = enabled
	return true
}

// Start begins the scheduler loop and executes due jobs.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopChan = make(chan struct{})
	s.mu.Unlock()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
			return
		case <-s.stopChan:
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
			return
		case now := <-ticker.C:
			s.checkSchedules(ctx, now)
		}
	}
}

// Stop halts the scheduler loop.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		close(s.stopChan)
	}
}

// checkSchedules evaluates schedules and triggers due jobs.
func (s *Scheduler) checkSchedules(ctx context.Context, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, schedule := range s.schedules {
		if schedule.Enabled && now.After(schedule.NextRun) {
			schedule.NextRun = now.Add(schedule.Interval)

			// Run job asynchronously
			go func(jobName string) {
				_, err := s.orchestrator.ExecuteJob(ctx, jobName, map[string]any{
					"triggered_by": "scheduler",
					"scheduled_at": now,
				})
				if err != nil {
					// In production, use proper logging
					fmt.Printf("Scheduled job %s failed: %v\n", jobName, err)
				}
			}(schedule.JobName)
		}
	}
}

// GetSchedules returns a snapshot of all configured schedules.
func (s *Scheduler) GetSchedules() []*Schedule {
	s.mu.RLock()
	defer s.mu.RUnlock()

	schedules := make([]*Schedule, 0, len(s.schedules))
	for _, sched := range s.schedules {
		// Return a copy to prevent external modification
		schedules = append(schedules, &Schedule{
			JobName:  sched.JobName,
			Interval: sched.Interval,
			Enabled:  sched.Enabled,
			NextRun:  sched.NextRun,
		})
	}
	return schedules
}

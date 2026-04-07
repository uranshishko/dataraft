package dataraft_test

import (
	"context"
	"fmt"
	"time"

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
		fmt.Println(r.Result)
	}
	return nil
}

func ExampleOrchestrator() {
	source := &intSource{}
	loader := &intLoader{}

	transform := func(r dataraft.Record[int]) (dataraft.TransformedRecord[int], error) {
		return dataraft.TransformedRecord[int]{ID: r.ID, Result: r.Data * 2}, nil
	}

	job := dataraft.NewTypedJob(
		"double",
		"double numbers",
		source,
		transform,
		loader,
		2,
		10,
		dataraft.WithTimeout[int, int](2*time.Minute),
	)

	orch := dataraft.NewOrchestrator(nil)
	_ = orch.RegisterJob(job)

	_, _ = orch.ExecuteJob(context.Background(), "double", nil)

	// Output:
	// 2
	// 4
	// 6
	// 8
	// 10
}

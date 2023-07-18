package utils

import (
	"github.com/hashicorp/go-multierror"
)

type CleanJob func() error

// NewCleanStack returns a new stack.
func NewCleanStack() *CleanStack {
	return &CleanStack{}
}

// Stack is a basic LIFO stack that resizes as needed.
type CleanStack struct {
	jobs  []CleanJob
	count int
}

// Push adds a node to the stack
func (clean *CleanStack) Push(job CleanJob) {
	clean.jobs = append(clean.jobs[:clean.count], job)
	clean.count++
}

// Pop removes and returns a node from the stack in last to first order.
func (clean *CleanStack) Pop() CleanJob {
	if clean.count == 0 {
		return nil
	}
	clean.count--
	return clean.jobs[clean.count]
}

// Cleanup runs the whole cleanup stack. In case of error it runs all jobs
// and returns the first error occurrence.
func (clean *CleanStack) Cleanup(err error) error {
	var errs error
	if err != nil {
		errs = multierror.Append(errs, err)
	}
	for clean.count > 0 {
		job := clean.Pop()
		err = job()
		if err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	return errs
}

package jobs

import "github.com/riverqueue/river"

func jobAttempt[T river.JobArgs](job *river.Job[T]) int64 {
	if job == nil || job.JobRow == nil {
		return 0
	}
	return int64(job.Attempt)
}

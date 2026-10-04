package console

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"yapl/internal/progress"
)

// JobState is a job's lifecycle stage.
type JobState string

const (
	JobQueued  JobState = "queued"
	JobRunning JobState = "running"
	JobDone    JobState = "done"
	JobFailed  JobState = "failed"
)

const (
	// DefaultJobRetention keeps finished jobs readable long enough for the
	// UI to show their outcome.
	DefaultJobRetention = 10 * time.Minute
	// maxQueuedJobs bounds the queue; a console never needs more.
	maxQueuedJobs = 64
)

var (
	ErrJobActive = errors.New("a job for this game is already queued or running")
	ErrQueueFull = errors.New("too many jobs queued")
)

// Job is a snapshot of an install or upgrade.
type Job struct {
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`
	Name     string    `json:"name"`
	State    JobState  `json:"state"`
	Stage    string    `json:"stage,omitempty"`
	Done     int64     `json:"done"`
	Total    int64     `json:"total"`
	Error    string    `json:"error,omitempty"`
	Created  time.Time `json:"created"`
	Finished time.Time `json:"finished,omitempty"`
	seq      int
}

type queuedJob struct {
	id  string
	run func(progress.Func) error
}

// Jobs runs jobs one at a time in submission order. Serial execution is
// deliberate: installs share runner.json and disk bandwidth.
type Jobs struct {
	mu        sync.Mutex
	jobs      map[string]*Job
	seq       int
	queue     chan queuedJob
	retention time.Duration
	now       func() time.Time
	done      chan struct{}
}

// NewJobs starts the worker. Close stops it.
func NewJobs(retention time.Duration) *Jobs {
	j := &Jobs{
		jobs:      map[string]*Job{},
		queue:     make(chan queuedJob, maxQueuedJobs),
		retention: retention,
		now:       time.Now,
		done:      make(chan struct{}),
	}
	go j.work()
	return j
}

// Close stops accepting jobs and waits for the worker to finish.
func (j *Jobs) Close() {
	close(j.queue)
	<-j.done
}

// Submit queues run for name. Only one job per name may be active.
func (j *Jobs) Submit(kind, name string, run func(progress.Func) error) (Job, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, existing := range j.jobs {
		if existing.Name == name && (existing.State == JobQueued || existing.State == JobRunning) {
			return *existing, ErrJobActive
		}
	}
	j.seq++
	job := &Job{ID: fmt.Sprintf("job-%d", j.seq), Kind: kind, Name: name, State: JobQueued, Total: -1, Created: j.now(), seq: j.seq}
	select {
	case j.queue <- queuedJob{id: job.ID, run: run}:
	default:
		return Job{}, ErrQueueFull
	}
	j.jobs[job.ID] = job
	return *job, nil
}

// Get returns a snapshot of one job.
func (j *Jobs) Get(id string) (Job, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.prune()
	job, ok := j.jobs[id]
	if !ok {
		return Job{}, false
	}
	return *job, true
}

// List returns snapshots of every retained job, oldest first.
func (j *Jobs) List() []Job {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.prune()
	out := make([]Job, 0, len(j.jobs))
	for _, job := range j.jobs {
		out = append(out, *job)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].seq < out[b].seq })
	return out
}

// prune drops finished jobs past their retention; callers hold mu.
func (j *Jobs) prune() {
	cutoff := j.now().Add(-j.retention)
	for id, job := range j.jobs {
		if !job.Finished.IsZero() && job.Finished.Before(cutoff) {
			delete(j.jobs, id)
		}
	}
}

func (j *Jobs) work() {
	defer close(j.done)
	for q := range j.queue {
		j.update(q.id, func(job *Job) { job.State = JobRunning })
		err := q.run(func(u progress.Update) {
			j.update(q.id, func(job *Job) {
				job.Stage, job.Done, job.Total = u.Label, u.Done, u.Total
			})
		})
		j.update(q.id, func(job *Job) {
			job.Finished = j.now()
			job.State = JobDone
			if err != nil {
				job.State = JobFailed
				job.Error = err.Error()
			}
		})
	}
}

func (j *Jobs) update(id string, f func(*Job)) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if job, ok := j.jobs[id]; ok {
		f(job)
	}
}

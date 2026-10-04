package console

import (
	"errors"
	"testing"
	"time"

	"yapl/internal/progress"
)

func waitFor(t *testing.T, jobs *Jobs, id string, state JobState) Job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if j, ok := jobs.Get(id); ok && j.State == state {
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
	j, _ := jobs.Get(id)
	t.Fatalf("job %s never reached %s; last %+v", id, state, j)
	return Job{}
}

func TestJobs(t *testing.T) {
	t.Run("given a job that reports progress when it runs then its progress is visible and it finishes done", func(t *testing.T) {
		// Given
		jobs := NewJobs(time.Minute)
		defer jobs.Close()
		release := make(chan struct{})

		// When
		job, err := jobs.Submit("install", "Doom", func(report progress.Func) error {
			report(progress.Update{Label: "proton/GE-Proton11-7", Done: 40, Total: 100})
			<-release
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}

		// Then
		running := waitFor(t, jobs, job.ID, JobRunning)
		deadline := time.Now().Add(time.Second)
		for running.Done != 40 && time.Now().Before(deadline) {
			running, _ = jobs.Get(job.ID)
		}
		if running.Done != 40 || running.Total != 100 || running.Stage != "proton/GE-Proton11-7" {
			t.Fatalf("running = %+v", running)
		}
		close(release)
		if done := waitFor(t, jobs, job.ID, JobDone); done.Finished.IsZero() {
			t.Fatalf("done = %+v", done)
		}
	})

	t.Run("given a job that fails then its error is recorded", func(t *testing.T) {
		jobs := NewJobs(time.Minute)
		defer jobs.Close()
		job, _ := jobs.Submit("install", "Doom", func(progress.Func) error { return errors.New("checksum mismatch") })
		if failed := waitFor(t, jobs, job.ID, JobFailed); failed.Error != "checksum mismatch" {
			t.Fatalf("failed = %+v", failed)
		}
	})

	t.Run("given a running job when another is submitted then the second waits in the queue", func(t *testing.T) {
		// Given
		jobs := NewJobs(time.Minute)
		defer jobs.Close()
		release := make(chan struct{})
		first, _ := jobs.Submit("install", "Doom", func(progress.Func) error { <-release; return nil })
		waitFor(t, jobs, first.ID, JobRunning)

		// When
		second, _ := jobs.Submit("install", "Quake", func(progress.Func) error { return nil })

		// Then
		if j, _ := jobs.Get(second.ID); j.State != JobQueued {
			t.Fatalf("second = %+v", j)
		}
		close(release)
		waitFor(t, jobs, second.ID, JobDone)
	})

	t.Run("given an active job for a game when another job for it is submitted then it is rejected", func(t *testing.T) {
		jobs := NewJobs(time.Minute)
		defer jobs.Close()
		release := make(chan struct{})
		defer close(release)
		jobs.Submit("install", "Doom", func(progress.Func) error { <-release; return nil })
		if _, err := jobs.Submit("upgrade", "Doom", func(progress.Func) error { return nil }); !errors.Is(err, ErrJobActive) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("given a finished job older than the retention when listed then it is gone", func(t *testing.T) {
		// Given
		jobs := NewJobs(time.Minute)
		defer jobs.Close()
		now := time.Now()
		jobs.now = func() time.Time { return now }
		job, _ := jobs.Submit("install", "Doom", func(progress.Func) error { return nil })
		waitFor(t, jobs, job.ID, JobDone)

		// When
		now = now.Add(2 * time.Minute)

		// Then
		if list := jobs.List(); len(list) != 0 {
			t.Fatalf("list = %+v", list)
		}
		if _, ok := jobs.Get(job.ID); ok {
			t.Fatal("expired job still retrievable")
		}
	})

	t.Run("given an unknown id then nothing is found", func(t *testing.T) {
		jobs := NewJobs(time.Minute)
		defer jobs.Close()
		if _, ok := jobs.Get("nope"); ok {
			t.Fatal("found a job that does not exist")
		}
	})
}

func TestJobsListOrder(t *testing.T) {
	t.Run("given several jobs submitted at the same instant then they are listed in submission order", func(t *testing.T) {
		jobs := NewJobs(time.Minute)
		defer jobs.Close()
		fixed := time.Now()
		jobs.now = func() time.Time { return fixed }
		for _, name := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K"} {
			jobs.Submit("install", name, func(progress.Func) error { return nil })
		}
		list := jobs.List()
		for i := 1; i < len(list); i++ {
			if list[i].Name < list[i-1].Name {
				t.Fatalf("out of order: %s before %s", list[i-1].Name, list[i].Name)
			}
		}
	})
}

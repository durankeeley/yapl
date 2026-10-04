package progress

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func collect() (*[]Update, Func) {
	var updates []Update
	return &updates, func(u Update) { updates = append(updates, u) }
}

func TestReader(t *testing.T) {
	t.Run("given a stream larger than the report step when read fully then updates are throttled and the last is complete", func(t *testing.T) {
		// Given
		total := int64(ReportStep*3 + 10)
		updates, report := collect()
		r := NewReader(bytes.NewReader(make([]byte, total)), "doom", total, report)

		// When
		n, err := io.CopyBuffer(io.Discard, r, make([]byte, 1024))

		// Then
		if err != nil || n != total {
			t.Fatalf("copied %d, %v", n, err)
		}
		if len(*updates) < 3 || len(*updates) > 5 {
			t.Fatalf("expected about one update per step plus a final one, got %d", len(*updates))
		}
		last := (*updates)[len(*updates)-1]
		if !last.Complete || last.Done != total || last.Total != total || last.Label != "doom" {
			t.Fatalf("final update = %+v", last)
		}
	})

	t.Run("given a nil report function when read then reading still works", func(t *testing.T) {
		r := NewReader(strings.NewReader("hello"), "x", 5, nil)
		if b, err := io.ReadAll(r); err != nil || string(b) != "hello" {
			t.Fatalf("read %q, %v", b, err)
		}
	})

	t.Run("given an empty stream when read then exactly one complete update is reported", func(t *testing.T) {
		updates, report := collect()
		io.ReadAll(NewReader(strings.NewReader(""), "x", 0, report))
		if len(*updates) != 1 || !(*updates)[0].Complete {
			t.Fatalf("updates = %+v", *updates)
		}
	})
}

func TestUpdatePercent(t *testing.T) {
	tests := []struct {
		name string
		u    Update
		want float64
	}{
		{"given half done then it is fifty percent", Update{Done: 50, Total: 100}, 50},
		{"given an unknown total then it is negative", Update{Done: 50, Total: -1}, -1},
		{"given a zero total then it is negative", Update{Done: 0, Total: 0}, -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.u.Percent(); got != tc.want {
				t.Fatalf("Percent = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPrinter(t *testing.T) {
	t.Run("given an interactive terminal then updates overwrite one line and end with a newline", func(t *testing.T) {
		// Given
		var buf bytes.Buffer
		p := NewPrinter(&buf, true)

		// When
		p(Update{Label: "doom", Done: 1 << 20, Total: 4 << 20})
		p(Update{Label: "doom", Done: 4 << 20, Total: 4 << 20, Complete: true})

		// Then
		out := buf.String()
		if !strings.HasPrefix(out, "\r") || !strings.Contains(out, "1.0 MB / 4.0 MB (25.0%)") || !strings.HasSuffix(out, "\n") {
			t.Fatalf("output = %q", out)
		}
		if strings.Count(out, "\n") != 1 {
			t.Fatalf("expected a single newline at completion, got %q", out)
		}
	})

	t.Run("given a log file then a line is written only per ten percent step", func(t *testing.T) {
		// Given
		var buf bytes.Buffer
		p := NewPrinter(&buf, false)
		total := int64(1000)

		// When
		for done := int64(0); done <= total; done += 10 {
			p(Update{Label: "doom", Done: done, Total: total, Complete: done == total})
		}

		// Then
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) < 10 || len(lines) > 12 {
			t.Fatalf("expected about 11 lines, got %d:\n%s", len(lines), buf.String())
		}
		if strings.Contains(buf.String(), "\r") {
			t.Fatal("log output must not use carriage returns")
		}
	})

	t.Run("given an unknown size then only the downloaded amount is shown", func(t *testing.T) {
		var buf bytes.Buffer
		NewPrinter(&buf, true)(Update{Label: "doom", Done: 3 << 20, Total: -1})
		if !strings.Contains(buf.String(), "3.0 MB downloaded") {
			t.Fatalf("output = %q", buf.String())
		}
	})

	t.Run("given an unknown size in a log then a line is written per hundred megabytes", func(t *testing.T) {
		var buf bytes.Buffer
		p := NewPrinter(&buf, false)
		for done := int64(0); done < 250*bytesPerMB; done += 10 * bytesPerMB {
			p(Update{Label: "doom", Done: done, Total: -1})
		}
		if n := strings.Count(buf.String(), "\n"); n != 3 {
			t.Fatalf("expected 3 lines (0, 100, 200 MB), got %d:\n%s", n, buf.String())
		}
	})

	t.Run("given a buffer then it is not detected as a terminal", func(t *testing.T) {
		if IsTerminal(&bytes.Buffer{}) {
			t.Fatal("buffer detected as terminal")
		}
	})
}

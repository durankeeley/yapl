// Package progress reports byte progress for long transfers. A Reader feeds
// Updates to any Func: the terminal printer here, or the console's job
// tracker.
package progress

import (
	"fmt"
	"io"
	"os"
)

const (
	// ReportStep is the minimum number of new bytes between updates, so tiny
	// reads don't flood the output.
	ReportStep = 256 << 10

	bytesPerMB = 1 << 20
	// logPercentStep and logUnknownStep throttle non-interactive output
	// (log files, pipes) to a readable number of lines.
	logPercentStep = 10
	logUnknownStep = 100 * bytesPerMB
	percentScale   = 100
)

// Update is one progress report. Total is -1 when the size is unknown.
type Update struct {
	Label    string
	Done     int64
	Total    int64
	Complete bool
}

// Percent returns 0–100, or -1 when the total is unknown.
func (u Update) Percent() float64 {
	if u.Total <= 0 {
		return -1
	}
	return float64(u.Done) * percentScale / float64(u.Total)
}

// Func receives progress updates.
type Func func(Update)

// Reader counts bytes read through it and reports them.
type Reader struct {
	r            io.Reader
	label        string
	total        int64
	done         int64
	lastReported int64
	report       Func
	finished     bool
}

// NewReader wraps r. A nil report function disables reporting.
func NewReader(r io.Reader, label string, total int64, report Func) *Reader {
	return &Reader{r: r, label: label, total: total, report: report, lastReported: -ReportStep}
}

func (p *Reader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if p.report == nil {
		return n, err
	}
	if err == io.EOF && !p.finished {
		p.finished = true
		p.report(Update{Label: p.label, Done: p.done, Total: p.total, Complete: true})
	} else if p.done-p.lastReported >= ReportStep {
		p.lastReported = p.done
		p.report(Update{Label: p.label, Done: p.done, Total: p.total})
	}
	return n, err
}

// IsTerminal reports whether w is an interactive character device.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// TerminalPrinter prints to w, overwriting one line on a terminal and
// writing throttled lines otherwise.
func TerminalPrinter(w io.Writer) Func {
	return NewPrinter(w, IsTerminal(w))
}

// NewPrinter prints updates to w. Interactive output redraws one line with
// carriage returns; non-interactive output writes a line per 10% (or per
// 100 MB when the size is unknown).
func NewPrinter(w io.Writer, interactive bool) Func {
	lastBucket := int64(-1)
	return func(u Update) {
		line := fmt.Sprintf("Downloading %s: %s", u.Label, describe(u))
		if interactive {
			fmt.Fprintf(w, "\r%s", line)
			if u.Complete {
				fmt.Fprintln(w)
			}
			return
		}
		bucket := u.Done / logUnknownStep
		if pct := u.Percent(); pct >= 0 {
			bucket = int64(pct) / logPercentStep
		}
		if bucket != lastBucket || u.Complete {
			lastBucket = bucket
			fmt.Fprintln(w, line)
		}
	}
}

func describe(u Update) string {
	done := float64(u.Done) / bytesPerMB
	if pct := u.Percent(); pct >= 0 {
		return fmt.Sprintf("%.1f MB / %.1f MB (%.1f%%)", done, float64(u.Total)/bytesPerMB, pct)
	}
	return fmt.Sprintf("%.1f MB downloaded", done)
}

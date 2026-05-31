package main

import (
	"fmt"
	"math"
	"os"
	"time"
)

const (
	netSpeedUpKBs    = 222.0 // KB/s upload  (Perl: $net_speed = 222)
	netSpeedDownKBs  = 74.0  // KB/s download (Perl: $net_speed_dl = 222.0/3.0)
	showProgressSec  = 10.0  // show bar only if estimated time > 10s
	progressBarWidth = 30
)

// estimateSec returns estimated transfer seconds for a given size and speed (KB/s).
func estimateSec(sizeBytes int64, speedKBs float64) float64 {
	return float64(sizeBytes) / 1024.0 / speedKBs
}

// runWithProgress runs doWork() in a goroutine and shows a progress bar if the estimated
// duration exceeds showProgressSec. Matches Perl create_progressbar_if_need_long_time +
// fun_progress_increase_ + Term::ProgressBar display.
func runWithProgress(estimatedSec float64, doWork func()) {
	if estimatedSec <= showProgressSec {
		doWork()
		return
	}

	done := make(chan struct{})
	go func() {
		doWork()
		close(done)
	}()

	start := time.Now()
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	fmt.Fprintln(os.Stderr, "")
	for {
		select {
		case <-done:
			elapsed := time.Since(start).Seconds()
			printProgressBar(1.0, elapsed, estimatedSec)
			fmt.Fprintln(os.Stderr, "")
			return
		case <-ticker.C:
			elapsed := time.Since(start).Seconds()
			// Advance to max 98% while still working (matches Perl: i*0.98 + rand)
			pct := math.Min(elapsed/estimatedSec*0.98, 0.98)
			printProgressBar(pct, elapsed, estimatedSec)
		}
	}
}

func printProgressBar(pct, elapsed, total float64) {
	filled := int(pct * progressBarWidth)
	if filled > progressBarWidth {
		filled = progressBarWidth
	}
	bar := make([]byte, progressBarWidth)
	for i := range bar {
		bar[i] = ' '
	}
	for i := 0; i < filled; i++ {
		bar[i] = '='
	}
	if pct < 1.0 && filled < progressBarWidth {
		bar[filled] = '>'
	}

	var eta string
	if pct >= 1.0 {
		eta = "done"
	} else {
		remaining := total - elapsed
		if remaining < 0 {
			remaining = 0
		}
		eta = fmt.Sprintf("%d:%02d", int(remaining)/60, int(remaining)%60)
	}

	fmt.Fprintf(os.Stderr, "\rProgress [%s] %3d%% ETA %s    ", string(bar), int(pct*100), eta)
}

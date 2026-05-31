package main

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/term"
)

const (
	progressMinBytes = 1 * 1024 * 1024 // show bar only for transfers > 1 MB
	// "Progress [" (10) + bar + "] 100% ETA done    " (18) = fixed overhead of 28 chars
	progressFixedOverhead = 28
	progressBarMinWidth   = 10
)

// termWidth returns the current terminal column count, defaulting to 80.
func termWidth() int {
	w, _, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

// barWidth returns how many chars the bar fill region should be for the current terminal.
func barWidth() int {
	w := termWidth() - progressFixedOverhead
	if w < progressBarMinWidth {
		w = progressBarMinWidth
	}
	return w
}

// runWithBytesProgress runs doWork() in a goroutine and shows a progress bar driven by
// actual bytes transferred (tracked atomically via countingConn). This gives precise,
// real-time ETA based on measured throughput — not a fixed speed estimate.
// Shows nothing if totalBytes < progressMinBytes.
func runWithBytesProgress(totalBytes int64, bytesXferred *int64, doWork func()) {
	if totalBytes < progressMinBytes {
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
			printBytesBar(atomic.LoadInt64(bytesXferred), totalBytes, elapsed)
			fmt.Fprintln(os.Stderr, "")
			return
		case <-ticker.C:
			elapsed := time.Since(start).Seconds()
			printBytesBar(atomic.LoadInt64(bytesXferred), totalBytes, elapsed)
		}
	}
}

func printBytesBar(xferred, total int64, elapsed float64) {
	if total <= 0 {
		return
	}
	pct := float64(xferred) / float64(total)
	if pct > 1.0 {
		pct = 1.0
	}

	bw := barWidth()
	filled := int(pct * float64(bw))
	if filled > bw {
		filled = bw
	}
	bar := make([]byte, bw)
	for i := range bar {
		bar[i] = ' '
	}
	for i := 0; i < filled; i++ {
		bar[i] = '='
	}
	if pct < 1.0 && filled < bw {
		bar[filled] = '>'
	}

	var eta string
	if pct >= 1.0 {
		eta = "done"
	} else if elapsed > 0.3 && xferred > 0 {
		// Real ETA from measured throughput: remaining_bytes / actual_speed
		speed := float64(xferred) / elapsed // bytes/sec
		remaining := float64(total-xferred) / speed
		if remaining < 0 {
			remaining = 0
		}
		eta = fmt.Sprintf("%d:%02d", int(remaining)/60, int(remaining)%60)
	} else {
		eta = "..."
	}

	fmt.Fprintf(os.Stderr, "\rProgress [%s] %3d%% ETA %-7s", string(bar), int(pct*100), eta)
}

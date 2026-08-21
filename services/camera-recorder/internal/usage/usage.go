// Package usage keeps the recorder's storage-usage metric up to date.
//
// The recorder writes video segments to disk forever, so "how much disk am I
// using" is something an operator needs to see on a dashboard rather than by
// running du on a server. Measuring it means walking the storage directory,
// which is far too expensive to do while answering a request, so it is done on
// a timer instead and the most recent answer is published as a gauge.
package usage

import (
	"context"
	"log"
	"time"

	"github.com/w0rxbend/instachron/services/camera-recorder/internal/storage"
)

// DefaultInterval is how often storage is measured when no other interval is
// given. Disk usage moves slowly compared with how long a walk of the storage
// tree takes, so there is nothing to gain from measuring it more often.
const DefaultInterval = 10 * time.Second

// Reporter is anything that can be told the current storage total. It is
// declared here, next to the code that calls it, rather than in the metrics
// package, so that this package depends on the one method it uses instead of on
// the whole metrics type — which also makes the loop below testable with a
// three-line fake.
type Reporter interface {
	SetStorageBytes(n int64)
}

// Measurer is the part of a storage backend this package needs: the ability to
// report its total size in bytes.
type Measurer interface {
	UsageBytes(ctx context.Context) (int64, error)
}

// Run measures storage immediately and then every interval until ctx is
// cancelled, publishing each result to reporter.
//
// A measurement that fails is logged and skipped rather than being fatal: the
// gauge keeps its previous value, which is a better answer than zero. Failures
// during shutdown are not logged at all, because a walk interrupted by the
// process stopping is expected rather than a problem worth reporting.
//
// Run measures once before waiting, so the gauge holds a real value from
// startup instead of reading zero for the first interval.
func Run(ctx context.Context, m Measurer, reporter Reporter, interval time.Duration, logger *log.Logger) {
	if interval <= 0 {
		interval = DefaultInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if n, err := m.UsageBytes(ctx); err == nil {
			reporter.SetStorageBytes(n)
		} else if ctx.Err() == nil {
			logger.Printf("storage usage: %v", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// compile-time check that the real storage backend satisfies Measurer.
var _ Measurer = (storage.Store)(nil)

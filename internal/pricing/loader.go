package pricing

import (
	"context"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DataKey is the ConfigMap key holding the price table.
const DataKey = "pricing.yaml"

// Loader reads the price-table ConfigMap with a short TTL cache so run
// completion never waits on the apiserver more than once per interval.
//
// It must be constructed with an UNCACHED reader (mgr.GetAPIReader()): the
// controller has no ConfigMap informer, so a Get through the manager's cached
// client would silently start a cluster-wide ConfigMap list+watch.
type Loader struct {
	Reader    client.Reader
	Name      string
	Namespace string
	TTL       time.Duration

	mu        sync.Mutex
	table     *Table
	err       error
	fetchedAt time.Time
}

// Load returns the current price table. A nil table with nil error means
// pricing is not configured. Errors (missing ConfigMap, malformed YAML) are
// cached for the TTL like successes, and callers must fail open: skip the
// estimate, never fail the run.
func (l *Loader) Load(ctx context.Context) (*Table, error) {
	if l == nil || l.Name == "" {
		return nil, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	ttl := l.TTL
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	if !l.fetchedAt.IsZero() && time.Since(l.fetchedAt) < ttl {
		return l.table, l.err
	}

	var cm corev1.ConfigMap
	if err := l.Reader.Get(ctx, types.NamespacedName{Name: l.Name, Namespace: l.Namespace}, &cm); err != nil {
		l.table, l.err, l.fetchedAt = nil, err, time.Now()
		return nil, err
	}
	t, err := ParseTable([]byte(cm.Data[DataKey]))
	l.table, l.err, l.fetchedAt = t, err, time.Now()
	return t, err
}

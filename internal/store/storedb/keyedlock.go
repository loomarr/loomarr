package storedb

import (
	"context"
	"sync"
)

// KeyedLock serializes one domain's read-decide-write sequences per key: a cross-replica advisory
// lock on Postgres, a process-local mutex per key on SQLite (which is single-instance, so the
// process is the whole lock domain). The zero value is ready to use.
//
// It exists so each cluster store owns its own lock domain. The domain's identity is the
// label/namespace pair passed to With plus the KeyedLock value itself: two domains that share a
// namespace would contend on Postgres, so every domain keeps its own namespace string, and each
// owns its own KeyedLock so its SQLite mutexes never touch another's. Entries live as long as the
// KeyedLock and are bounded by the household's key space (§1).
//
// Not for every lock: proposal approval orders requests with a context-aware waiter and a
// transaction-bound Postgres lock (internal/store/approval.go), a different contract that stays
// with its owner.
type KeyedLock struct {
	local sync.Map // key -> *sync.Mutex
}

// With runs fn holding the lock for key. label is the human name in error text; namespace is the
// Postgres advisory-lock namespace prefix.
func (k *KeyedLock) With(
	ctx context.Context,
	h AdvisoryLocker,
	label, namespace, key string,
	fn func(context.Context) error,
) error {
	if h.Dialect() == DialectPostgres {
		return h.WithAdvisoryLock(ctx, label, namespace, key, fn)
	}
	value, _ := k.local.LoadOrStore(key, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	return fn(ctx)
}

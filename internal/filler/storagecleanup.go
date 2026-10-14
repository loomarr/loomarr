package filler

// StorageCleanupPreview is the server-owned answer to “what can Loomarr safely remove?”.
// It includes only terminal private staging proven disposable by the acquisition cleaner.
type StorageCleanupPreview struct {
	Items int
	Bytes int64
}

// StorageCleanupResult reports one explicit cleanup attempt. Remaining is a fresh preview after
// deletion, not arithmetic derived from the earlier preview, so concurrent filesystem changes are
// represented honestly.
type StorageCleanupResult struct {
	RemovedItems int
	RemovedBytes int64
	FailedItems  int
	Remaining    StorageCleanupPreview
}

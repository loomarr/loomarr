package media.loomarr.tv.discovery;

/** Tracks two foreground unicast sweeps without spending one before a LAN plan exists. */
final class UnicastRetryPolicy {
  private final long sweepIntervalMs;
  private final long recheckIntervalMs;
  private int remaining = 2;
  private long nextAttemptMs;

  UnicastRetryPolicy(long sweepIntervalMs, long recheckIntervalMs) {
    this.sweepIntervalMs = sweepIntervalMs;
    this.recheckIntervalMs = recheckIntervalMs;
  }

  boolean isDue(long nowMs) {
    return remaining > 0 && nowMs >= nextAttemptMs;
  }

  void unavailable(long nowMs) {
    nextAttemptMs = nowMs + recheckIntervalMs;
  }

  void sent(long nowMs) {
    remaining -= 1;
    nextAttemptMs = nowMs + sweepIntervalMs;
  }

  int remaining() {
    return remaining;
  }
}

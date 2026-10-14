# 0022. Pause is shared time-shift

- **Date:** 2026-08-17
- **Status:** accepted
- **Supersedes:** a private per-viewer playback stack

## Context

Viewers expect to pause live television. A per-viewer pause would need a per-viewer encode or
buffer, which breaks one encode per Channel and multiplies cost by audience.

## Decision

Pause freezes the viewer's position inside one shared, bounded DVR window that the Channel's packager
already keeps (V60). Resume continues from that position while it remains inside the fifteen-minute
horizon; otherwise the player returns to live and says the point expired. **Go Live** is one explicit
action, and tuning always joins live.

## Consequences

- Pausing any number of viewers creates no extra encoders; only a Channel with a viewer holds
  segments on disk.
- The horizon is one server constant used by the packager and the Watch timeline, not a setting.
- Client players must keep their back buffers beyond the horizon so a promised position is not
  discarded locally.

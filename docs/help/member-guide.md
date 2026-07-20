# Member guide

For everyone who describes channels and watches them.

## What you can do

- **Members** — describe channels, see every proposal and channel, watch. You can't
  approve (approving starts downloads, so it's admin-only).
- **Admins** — also approve/deny, manage users, and set up integrations.

You sign in with your media-server password, once an admin has imported your account.

## Writing a good intent

Describe the vibe — you don't need to name exact titles. Loomarr matches against your
actual library.

- **Good:** "90s Saturday morning cartoons for the kids" · "cozy British murder mysteries"
  · "high-energy 80s action"
- **Add constraints** if they matter: an era, a tone, a runtime target, titles to include
  or exclude.
- **"For the kids"** caps the content rating — nothing above it will play.

The more specific the theme, the tighter the channel. "Good movies" grounds poorly.

## After you submit

1. The suggester runs — searching, reasoning, scoring (tens of seconds on a local model).
2. A proposal appears: what you have, plus what's missing, each with a reason.
3. An admin approves — which creates the channel and starts acquiring the rest.

## Reading channel status

- **Building** — being set up; give it a moment.
- **Live** — playing. Missing titles show as commercials until they arrive, then swap in.
- **Drifted** — something it was playing vanished from the library; Loomarr repairs it on
  the next refresh.

Channels keep maintaining themselves — nothing to do once they're live.

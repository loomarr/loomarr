package releaseverify

const cacheCleanupWorkflowCommand = "set -uo pipefail\n" +
	"# `|| true` on the list: a PR with no caches is the normal case for docs-only\n" +
	"# work, and it must not fail the job.\n" +
	"ids=$(gh api \"repos/$REPO/actions/caches\" --paginate \\\n" +
	"        -q \".actions_caches[] | select(.ref==\\\"$REF\\\" or (.ref | startswith(\\\"$QUEUE_REF_PREFIX\\\"))) | .id\" 2>/dev/null || true)\n" +
	"if [ -z \"$ids\" ]; then\n" +
	"  echo \"no caches for $REF or $QUEUE_REF_PREFIX*\"\n" +
	"  exit 0\n" +
	"fi\n" +
	"for id in $ids; do\n" +
	"  # Best-effort per cache: a 404 here means something else already deleted it\n" +
	"  # (a concurrent run, or GitHub's own expiry), which is success, not failure.\n" +
	"  if gh api -X DELETE \"repos/$REPO/actions/caches/$id\" >/dev/null 2>&1; then\n" +
	"    echo \"deleted $id\"\n" +
	"  else\n" +
	"    echo \"could not delete $id (already gone?)\"\n" +
	"  fi\n" +
	"done\n"

const appleCacheFingerprintCommand = `echo "fingerprint=$(./scripts/apple-compilation-cache.sh fingerprint)" >> "$GITHUB_OUTPUT"`

const codeQLImpactCommand = `set -uo pipefail
if [ "$EVENT_NAME" = schedule ] || [ "$EVENT_NAME" = workflow_dispatch ]; then
  ./scripts/codeql-impact.sh --all >> "$GITHUB_OUTPUT"
elif [ -z "${BASE:-}" ] || [ "$BASE" = "0000000000000000000000000000000000000000" ] \
  || ! git cat-file -e "$BASE^{commit}" 2>/dev/null; then
  echo "no usable CodeQL diff base ($BASE) — selecting every language"
  ./scripts/codeql-impact.sh --all >> "$GITHUB_OUTPUT"
elif ! git diff --name-only "$BASE" HEAD | ./scripts/codeql-impact.sh >> "$GITHUB_OUTPUT"; then
  echo "CodeQL impact classification failed — selecting every language"
  ./scripts/codeql-impact.sh --all >> "$GITHUB_OUTPUT"
fi
`

const appleCachePublisherPreflightCommand = `set -euo pipefail
gh api "repos/$REPO/actions/cache/usage" > "$RUNNER_TEMP/apple-cache-usage.json"
./scripts/apple-compilation-cache.sh admit-capacity "$RUNNER_TEMP/apple-cache-usage.json"
`

const appleCacheConsumerPrepareCommand = `set -euo pipefail
mode=cold
store="$RUNNER_TEMP/apple-compilation-cache-store"
archive="$RUNNER_TEMP/apple-compilation-cache.tar.zst"
if [[ -f "$archive" ]] && ./scripts/apple-compilation-cache.sh restore "$archive" "$store"; then
  mode=warm
else
  rm -f "$archive"
fi
echo "mode=$mode" >> "$GITHUB_OUTPUT"
echo "store=$store" >> "$GITHUB_OUTPUT"
`

const appleCachePublisherSeedCommand = `set -euo pipefail
archive="$RUNNER_TEMP/apple-compilation-cache.tar.zst"
seed="$RUNNER_TEMP/apple-compilation-cache-seed.tar.zst"
if [[ -f "$archive" ]]; then
  mv "$archive" "$seed"
  echo "archive=$seed" >> "$GITHUB_OUTPUT"
else
  echo "archive=" >> "$GITHUB_OUTPUT"
fi
`

const appleCachePublisherAdmissionCommand = `set -euo pipefail
gh api "repos/$REPO/actions/cache/usage" > "$RUNNER_TEMP/apple-cache-usage.json"
./scripts/apple-compilation-cache.sh admit-save \
  "$RUNNER_TEMP/apple-compilation-cache.tar.zst" \
  "$RUNNER_TEMP/apple-cache-usage.json"
`

const appleCachePublisherRetentionCommand = `set -euo pipefail
inventory="$RUNNER_TEMP/apple-cache-inventory.json"
gh api "repos/$REPO/actions/caches?per_page=100&ref=refs/heads/main" > "$inventory"
while read -r id; do
  [[ -n "$id" ]] || continue
  gh api -X DELETE "repos/$REPO/actions/caches/$id"
done < <(./scripts/apple-compilation-cache.sh retention-plan \
  "$inventory" "$PREFIX" refs/heads/main 1)
`

const androidCCachePromotionRetentionCommand = `set -euo pipefail
gh api -X DELETE "repos/$REPO/actions/artifacts/$ARTIFACT_ID"
inventory="$RUNNER_TEMP/android-ccache-inventory.json"
gh api "repos/$REPO/actions/caches?per_page=100&ref=refs/heads/main" > "$inventory"
ids=$(./scripts/android-ccache-promotion.sh retention-plan \
  "$inventory" refs/heads/main android-tv-ccache-v2-Linux-ccache-4.14- "$KEEP_KEY")
while read -r id; do
  [[ -n "$id" ]] || continue
  gh api -X DELETE "repos/$REPO/actions/caches/$id"
done <<< "$ids"
`

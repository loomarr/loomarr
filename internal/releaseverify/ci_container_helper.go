package releaseverify

import (
	"fmt"
	"os"
)

const containerImageHelperContract = `#!/usr/bin/env bash
set -u

if [[ $# -ne 1 ]]; then
  echo "usage: $0 IMAGE" >&2
  exit 2
fi

image=$1
if docker image inspect "$image" >/dev/null 2>&1; then
  exit 0
fi

pull_status=1
for attempt in 1 2 3 4 5; do
  if docker pull "$image"; then
    exit 0
  else
    pull_status=$?
  fi
  if [[ $attempt -lt 5 ]]; then
    sleep 2 || exit $?
  fi
done

exit "$pull_status"
`

const playwrightContainerRunnerContract = `#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 ensure|visual|visual-update|e2e|e2e-update|tuner-e2e" >&2
  exit 2
fi

readonly image=mcr.microsoft.com/playwright:v1.62.0-noble
readonly mode=$1

if [[ $mode == ensure ]]; then
  exec ./scripts/ensure-container-image.sh "$image"
fi

repo_root=$(pwd -P)
readonly repo_root
host_user=$(/usr/bin/id -u):$(/usr/bin/id -g)
readonly host_user
readonly runner=${GITHUB_ACTIONS:-}
docker_args=(
  run --rm --ipc=host
  --user "$host_user"
  -e HOME=/tmp
  -e CI=1
  -e "GITHUB_ACTIONS=$runner"
)

case "$mode" in
  visual)
    shard_args=()
    if [[ -n ${LOOMARR_PLAYWRIGHT_SHARD:-} ]]; then
      if [[ ! $LOOMARR_PLAYWRIGHT_SHARD =~ ^--shard=([1-9][0-9]*)/([1-9][0-9]*)$ ]] ||
        ((BASH_REMATCH[1] > BASH_REMATCH[2])); then
        echo "invalid LOOMARR_PLAYWRIGHT_SHARD: $LOOMARR_PLAYWRIGHT_SHARD" >&2
        exit 2
      fi
      shard_args=("$LOOMARR_PLAYWRIGHT_SHARD")
    fi
    exec docker "${docker_args[@]}" -v "$repo_root/web:/work" -w /work/apps/web "$image" \
      node_modules/.bin/playwright test "${shard_args[@]}"
    ;;
  visual-update)
    exec docker "${docker_args[@]}" -v "$repo_root/web:/work" -w /work/apps/web "$image" \
      node_modules/.bin/playwright test --update-snapshots
    ;;
  e2e)
    exec docker "${docker_args[@]}" -v "$repo_root:/work" -w /work/web/apps/web "$image" \
      node_modules/.bin/playwright test --config=playwright.e2e.config.ts
    ;;
  e2e-update)
    exec docker "${docker_args[@]}" -v "$repo_root:/work" -w /work/web/apps/web "$image" \
      node_modules/.bin/playwright test --config=playwright.e2e.config.ts --update-snapshots
    ;;
  tuner-e2e)
    exec docker "${docker_args[@]}" -v "$repo_root:/work" -w /work/web/apps/web "$image" \
      node_modules/.bin/playwright test --config=playwright.tuner.config.ts
    ;;
  *)
    echo "unknown Playwright container mode: $mode" >&2
    exit 2
    ;;
esac
`

// verifyContainerImageHelper admits one reviewed shell program. Exact source
// comparison is deliberately fail closed: shell is too dynamic for a partial
// parser to prove the inspect, retry, wait, and final-status semantics.
func verifyContainerImageHelper(helperPath string) error {
	source, err := os.ReadFile(helperPath)
	if err != nil {
		return err
	}
	if string(source) != containerImageHelperContract {
		return fmt.Errorf("helper must match the reviewed bounded-acquisition program")
	}
	return nil
}

func verifyPlaywrightContainerRunner(path string) error {
	source, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(source) != playwrightContainerRunnerContract {
		return fmt.Errorf("runner must match the reviewed override-safe Playwright container program")
	}
	return nil
}

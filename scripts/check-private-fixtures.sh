#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# SHA-256 fingerprints of the exact values removed during the first-beta fixture audit. The
# private values themselves must not be retained here, even as adjacent shell fragments. Scans
# normalize case so a case-only spelling change cannot bypass the guard. Candidate extraction is
# deliberately limited to the value shapes found in the captures; this is a regression guard,
# not a general-purpose secret scanner.
FINGERPRINTS=(
  "425997118cfa504677beaa951a1218d8c17d55b9429cf9bafb5a884cf8338c86|private Live TV address"
  "8b51baea68b18710dd1d3ffbd6c84816e527cb485baeb58ff4252fdc5630deac|private tuner address"
  "7ab56f45c3f95a0a428f5e212f6c4026264cca2f088727bf9da59a72bd521e8d|private media-server address"
  "8aaaba99ee3a238b0eeccf0b3011d8b1937fe03a21ada49ef3d7eb3cc12eb001|private app address"
  "8d9d7f390ea02b10f2a03144fe473353ad7c7d62f490fbdf845d5c10f3494261|live media-server hostname"
  "6a994e8e0c22937d7d0dd67072119cbd72effa8b103c89b2d46e74452614ec8e|captured personal email"
  "8d7fdc3890c51640f2128f256dc908571f804209a8db5c006095bfa4e6a4f4dc|captured personal email"
  "aa772ab6afdf0a02770c480967ea5b8a904d33ca4ec252e376b894a3d2582782|captured personal email"
  "c8e04438762f6a955618e7db16aa53da449e837192043d73614fb398caa0ed5d|captured personal email"
  "fb613559f9b231237dfaea0a5939f9f0fadfa0d6e7b3346cc57b4e309b5615c8|captured personal email"
  "0edadbb013fd8453a996ca26e9f92f8f5d994506445a87e831a37024ea732e6b|captured server id"
  "06f11cb3c2beec39ad0a9e46a5f9e87d7cdf8a4ce95534a1bf581151708f87db|captured user id"
  "78e8041a3a3752e2b4da00b6f83ceb0640567fb214b69a8fbf4f425aa1eefb5f|captured user id"
  "e786b998d323c9c7ef2965f1717187ead6757a535a63e766df6ba1a15ea981b8|captured user id"
  "50a577f13d741d537378a24ef80a4f7095e9a89fff6143ce36f1117eebc975b6|captured user id"
  "22133196bf1f3ff3d29de6459bbb22d703181d8e8a52f65d2c621d996d1045d1|captured user id"
  "8caaf9856067b742d1961c7b9aa949770844e01da60a2f8e3f22a85e7e18cf68|captured user id"
  "8e7921d507bfc7e7010fd19b593f00b22d34092bce5ae52eb22c4fa19fe1d68a|captured user id"
  "322766a25bb4b8a41ca2db239c2f8ef9b2754a88b7358612d7dfb01731647fad|captured user id"
  "f9c9f50ba97edb1411b46dfdd4579b0617c43c5d9ef1d2c6ad7048113e996cd3|captured user id"
  "d870787f780f9677284903ff6ca105138521b27599d71dda25ff307ccb03f8c8|captured user id"
  "37a5a659d18cad9b5b22833a2a0750d602ddfc25773b7b8ff3d07eb3aeb0fdf6|captured tuner id"
  "f28aa2bcef2dc219ec3064f6d221ebc81406e614956f45cb80e811f94f3b6cb6|captured listing id"
  "58bfb7b2f7fffd4a380ff416b212b68cca6aca5a9fccc6ee964ccca53f834487|captured tuner id"
  "632efcadd239c2404d502785c14f781e83decb35e3ed51dbf125c22bf71ae63b|captured hardware address"
  "219ddd11b35b4753407522cc32713e8c8d0842e27c269ae1e459d53c39eb0062|captured hardware address"
  "d15489fdc3b21f4f419e58947481d57dde70440d270294c2e039ee3fc76c2d6b|captured hardware address"
  # Household names and profile PINs are checked only where those captured response fields live.
  # A bare given name or year-like number is legitimate elsewhere in source and documentation.
  "911f3d3ea12da90601fe05689e13d712551f1c2fe175eae213eb01b6be896b06|captured household name"
  "6d219eef9ed1213b994241c031488503c3843b8dc91bc26aedd24b70228e1293|captured household name"
  "e010a14d62b51da432ee72399a6ba09070a5ae47bd1f5bfd18e8d586f0aa813a|captured household name"
  "444a131817a4c64fd0f851bfa0067b92eba75a003d37a62106c6cb1af8a1e527|captured household name"
  "26e1400d63222defb5c64de18a88a3ad166683982ee36ac11171f4c87627ac45|captured household name"
  "89856140ce2e0cdcdcf0f0969dcfa32cf912403707a2c8099e0961c9d37c7d9f|captured household name"
  "74eaf15c4394e49604de0c3c026617b1c9cc5926f1eb1308fea3b8dfed80433d|captured household name"
  "1567dae7f504ee2c440d992e6b2a180eec3c902cdb326d50a19e5211a1662645|captured household name"
  "a49169515985dcb3fb79324fe794e0b8cd871c7083641064cea364556a0be1c7|captured household name"
  "4c9ab350808e40e341c98c33b12a088b2e9f0c276918bc86232c6c21c089ffa0|captured household name"
  "f5b0edbf69c1c6caf8be0ac084598aa3b6aec96e1573291699212190851ac6d1|captured household name"
  "9eb1d8d316b63e83a9ad4b72a1d277b7cc0fa66483ba8cc3af2a77e9b585681f|captured account name"
  "75ef0a3922ff6e59f3a9a9981f56c07e4e1a6f897647c2c9f80ad222d572839c|captured account name"
  "dd26ef44f54fcea3918bc856801f616ad5854104923f5ba9b8da528dc5143c1d|captured profile PIN"
  "3c20e4ea1540e8a9e4f3792764b6b1e88c3cb053dbed9a8142ec69f24e8e5321|captured profile PIN"
)

fixture_paths=(
  internal/testkit/fixtures/emby/auth_success_response.json
  internal/testkit/fixtures/emby/users_list.json
  internal/testkit/fixtures/seerr/request_available_201.json
  internal/testkit/fixtures/seerr/request_repeat.json
)

fingerprint() {
  local value
  value="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"
  if command -v sha256sum >/dev/null 2>&1; then
    printf '%s' "$value" | sha256sum | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    printf '%s' "$value" | shasum -a 256 | awk '{print $1}'
  else
    echo 'private-fixture guard: sha256sum or shasum is required' >&2
    return 2
  fi
}

tracked_candidates() {
  local regex="$1"
  shift
  local hits status
  if hits="$(git grep -h -I -o -E -e "$regex" -- "$@")"; then
    printf '%s\n' "$hits"
    return 0
  else
    status=$?
  fi
  if ((status == 1)); then
    return 0
  fi
  printf 'private-fixture guard: git grep failed with status %d\n' "$status" >&2
  return "$status"
}

scan_candidates() {
  local regex="$1"
  shift
  local candidates candidate digest row expected label

  candidates="$(tracked_candidates "$regex" "$@")"
  [[ -z "$candidates" ]] && return 0
  while IFS= read -r candidate; do
    digest="$(fingerprint "$candidate")"
    for row in "${FINGERPRINTS[@]}"; do
      expected="${row%%|*}"
      [[ "$digest" == "$expected" ]] || continue
      label="${row#*|}"
      printf 'private-fixture guard: %s fingerprint remains (%s)\n' "$label" "$digest" >&2
      fail=1
    done
  done < <(printf '%s\n' "$candidates" | sort -fu)
}

fail=0
scan_candidates '([0-9]{1,3}\.){3}[0-9]{1,3}' .
scan_candidates '[[:alnum:]._%+-]+@[[:alnum:].-]+\.[[:alpha:]]{2,}' .
scan_candidates '[[:alnum:]_-]+(\.[[:alnum:]_-]+){2,}' .
scan_candidates '[[:xdigit:]]{32}' .
scan_candidates '[[:xdigit:]]{12}' .
scan_candidates '"(Name|jellyfinUsername|displayName|ProfilePin)"[[:space:]]*:[[:space:]]*"[^"]*"' "${fixture_paths[@]}"

if ((fail)); then
  exit 1
fi

echo "private-fixture guard: captured private literals are absent"

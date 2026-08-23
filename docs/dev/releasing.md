# Publishing a release

Server image publication and GitHub Release publication intentionally run in separate workflows from
the same `v*` tag. The image workflow owns package write and keyless-signing permissions. The release
notes workflow owns only `contents: write`; it cannot build, sign, or promote an image.

## One-time repository setup

Add a dedicated OpenRouter key as the masked Actions secret used only by release notes:

```sh
gh secret set OPENROUTER_RELEASE_API_KEY --repo loomarr/loomarr
```

The default model is `openai/gpt-5-mini`. To change it without editing the workflow, set the optional
repository variable `RELEASE_NOTES_MODEL` to an OpenRouter model that supports strict structured
outputs. The release process is designed for a small classification call, not model-authored prose.

## Preview before tagging

Authenticate `gh`, export the release-specific key locally, and generate a preview for the proposed
tag. The tag may already exist or be a commit-ish accepted by GitHub's generated-notes endpoint.

```sh
export OPENROUTER_RELEASE_API_KEY='...'
make release-notes-preview TAG=v0.2.0 PREVIOUS_TAG=v0.1.0-beta.1
```

The default output is `.artifacts/release-notes-v0.2.0.md`. Set `OUTPUT=/path/to/notes.md` to choose
another destination. Do not commit or paste the key. A tag-specific
`docs/release/<tag>.md` file, when present, is prepended for human-authored framing and known
limitations.

## What the model can and cannot do

The helper first asks GitHub to generate the exact merged-PR list, contributor list, and compare link.
It sends only each PR number and title to OpenRouter. The response must assign every number exactly
once to one of these fixed sections:

- New Features
- Improvements
- Bug Fixes
- Security Fixes
- Documentation
- Dependencies
- Maintenance

Repository code renders GitHub's original bullet for each assignment. It rejects unknown JSON fields,
invented PRs, duplicate PRs, omitted PRs, malformed output, and unrecognized GitHub change lines. It
retries inference three times and then fails without creating a GitHub Release. It never silently
publishes uncategorized or model-authored notes.

## Tag and verify

Push the protected version tag only after the required commit gates are green. Both release workflows
start from that tag. If OpenRouter is temporarily unavailable, rerun the failed **Release notes**
workflow after service recovers; the separately hardened image publication is unaffected.

After both workflows finish, verify the GitHub Release body, the GHCR manifest, signature, SBOM, and
provenance against the tagged commit. The tag-specific header remains the place to state limitations
that cannot be derived from pull requests.

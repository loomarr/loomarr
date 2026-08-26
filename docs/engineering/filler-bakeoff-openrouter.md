# OpenRouter filler bakeoff

This is the paid, label-blind capture boundary for filler admission. It does not download media,
read human labels, score results, or authorize production behavior. Run it only after the 300-case
certification manifest and its evidence-packet JSONL are locked.

## Inputs

- A schema-v4 certification manifest. Only its named `development` or `holdout` split is executed.
- One packet JSON object per line. Packet case IDs and hashes must match that split exactly.
- A corpus root containing the packet's hashed frame, audio, and video derivatives.
- A schema-v1 OpenRouter snapshot no more than 24 hours older than the run, created from the exact
  candidate models through `make filler-openrouter-snapshot`.
- A schema-v1 run config containing the immutable run identity, admission policy, and ordered routes.
- `OPENROUTER_API_KEY` in the environment. Credentials never enter the config or output ledger.

The run config is strict: unknown or trailing fields fail. One model comparison is one independently
named config and output ledger; do not put several candidate models behind fallback routing. Use
concrete namespaced model IDs and the exact upstream provider name captured in the capability
snapshot. `promptVersion` is fixed to `filler-evidence-openrouter-v1` because that name identifies
the prompt and output schema compiled into the adapter.

```json
{
  "schemaVersion": 1,
  "run": {
    "profile": "candidate-a-holdout",
    "evaluationSplit": "holdout",
    "evidenceVersion": "filler-evidence-v1",
    "promptVersion": "filler-evidence-openrouter-v1",
    "taxonomyVersion": "filler-taxonomy-v1",
    "policyVersion": "filler-admission-v1",
    "rolePolicyVersion": "filler-roles-candidate-a-v1",
    "capabilitySnapshot": "<snapshot SHA-256>",
    "priceSnapshot": "<the same snapshot SHA-256>",
    "generatedAt": "YYYY-MM-DDTHH:MM:SSZ",
    "maxRequests": 300,
    "maxSpendNanoUsd": 5000000000,
    "maxConcurrency": 1
  },
  "policy": {
    "version": "filler-admission-v1",
    "taxonomyVersion": "filler-taxonomy-v1",
    "allowedProducts": ["example-product"],
    "allowedContentRoles": ["commercial", "bumper", "psa", "station_id"],
    "knownSensitiveFlags": ["adult", "violence"],
    "prohibitedFlags": ["adult"]
  },
  "routes": [
    {
      "class": "text",
      "role": "filler_text",
      "rung": "text",
      "provider": "openrouter",
      "model": "vendor/concrete-model-id",
      "upstreamProviderSlug": "exact-provider/variant",
      "upstreamProvider": "Exact Provider Name",
      "modalities": ["text"],
      "structuredOutput": true,
      "requireZdr": true,
      "allowFallbacks": false,
      "maxChargeNanoUsd": 10000000,
      "maxAttempts": 1,
      "escalateOn": ["missing_content_role"]
    }
  ]
}
```

The ceiling values above illustrate units, not approved budgets. Lock them from the capability and
price snapshots before a real run. Add frame, video, or premium rungs only in the typed cascade order
accepted by the runner and only for their named escalation reasons.

`upstreamProviderSlug` is the exact routing selector copied from the endpoint snapshot;
`upstreamProvider` is the distinct human provider identity expected in router metadata. They are
both required because OpenRouter does not report the selector slug as the selected provider name.

## Capture and replay

First freeze the exact candidate set. The ZDR endpoint list is authenticated even though this step
performs no inference or chargeable completion. Use the eight concrete issue #555 candidates unless
that issue is amended before the run; the command sorts them and refuses aliases or duplicates.

```sh
export OPENROUTER_API_KEY='...'
export LOOMARR_FILLER_OPENROUTER_MODELS='google/gemini-3.7-flash,qwen/qwen3.8-27b,google/gemma-4-26b-a4b-it,openai/gpt-4.1-mini,openai/gpt-5-mini,anthropic/claude-sonnet-5,google/gemini-3.1-flash-lite,qwen/qwen2.5-vl-72b-instruct'
export LOOMARR_FILLER_OPENROUTER_SNAPSHOT=/absolute/path/openrouter-snapshot.json
make filler-openrouter-snapshot
```

The command records the retrieval time itself. Copy the printed SHA-256 into both run snapshot fields
and choose only endpoints marked `zdr: true` in that file. The capture command re-hashes and validates
the file; a name alone is not evidence.

```sh
export OPENROUTER_API_KEY='...'
export LOOMARR_FILLER_BAKEOFF_MANIFEST=/absolute/path/manifest.json
export LOOMARR_FILLER_BAKEOFF_PACKETS=/absolute/path/packets.jsonl
export LOOMARR_FILLER_BAKEOFF_CONFIG=/absolute/path/candidate-a.json
export LOOMARR_FILLER_BAKEOFF_SNAPSHOT=/absolute/path/openrouter-snapshot.json
export LOOMARR_FILLER_BAKEOFF_CORPUS_ROOT=/absolute/path/corpus
export LOOMARR_FILLER_BAKEOFF_PREDICTIONS=/absolute/path/candidate-a.predictions.jsonl
make filler-bakeoff-openrouter
```

The adapter performs exactly one provider attempt for each reserved rung. It sends only the
route-selected signals, pins the upstream provider, disables fallback, requires supported strict
structured output, denies provider data collection, requires ZDR routing, and opts into routing
metadata. A response is accepted only when its model and selected upstream match the reservation,
its metadata reports one attempt, and `usage.cost` is present as exact USD decimal text. Each fact
must name a supplied signal ID. A semantic abstention is captured as a successful charged step with
one reason and no evidence; transport, schema, or accounting failures remain operational failures.

Score each captured ledger separately with `make filler-eval-cert`, using the same run identity and
ceilings. Compare the resulting reports by the predeclared quality, coverage, safety-slice, latency,
and cost gates. Do not merge ledgers, tune against holdout labels, or promote a route based only on a
catalog capability claim.

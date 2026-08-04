# whisper-cli language-detection captures

Real output from the **vendored** binary and model, not hand-written JSON. CLAUDE.md's
"fixtures are pinned truth" rule is why: a parser written against remembered field names works
until it meets the tool.

Captured 2026-08-03 from `loomarr:v40-test`, built from this repo's Dockerfile.

| | |
| --- | --- |
| Binary | `whisper-cli`, whisper.cpp `v1.9.1` |
| Model | `ggml-tiny.bin` (multilingual), rev `5359861c…`, sha256 `be07e048…` |
| Arguments | `-m <model> -f <span>.wav -l auto -oj -of <out> -np` — the exact set `WhisperLanguage.DetectLanguage` builds |
| Audio | 16kHz mono wav, the 1s–11s span `LanguageSpan(29814)` asks for |

## The two clips

- **`lang_es.json`** — a Spanish Coca-Cola advert from archive.org
  (`spanishrevolution-ElmejoranunciodeCocaCola-1LE-AHD7l80`), which the source itself declares as
  `language: spanish`. A *genuine* foreign clip rather than something synthesised to match our own
  expectations.
- **`lang_en.json`** — a real clip from the dev drop-folder, same span rule.

## What they pin

**`result.language` is the DETECTED language; `params.language` is what was REQUESTED.** Both
captures show `params.language: "auto"`, so a parser reading the wrong field returns `"auto"` for
every clip on earth — and the gate silently never fires. That is a no-op feature, not a crash,
which is exactly why it needs a fixture rather than a comment.

They are also the evidence that vendoring a **second** model was necessary. The shipped
`ggml-small.en.bin` is English-only and does not perform language identification at all — it would
have answered `en` for the Spanish capture.

## Trimming

The `transcription` array is cut to two utterances. The parser only asks *"was anything said"*, so
the rest is bulk; everything the parser reads is verbatim.

Regenerate by rebuilding the image and re-running the arguments above.

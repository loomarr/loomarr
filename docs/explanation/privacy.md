# Privacy

**For:** anyone deciding what Loomarr may send outside the home network.
**You'll get:** every outside service Loomarr can contact, what it sends, and how to turn it off.

Loomarr runs on your hardware and keeps its data in your database. Your media server, Seerr and
the *arrs are on your network, so talking to them never leaves it. Nothing is uploaded to the
Loomarr project: there's no telemetry, no account and no update check.

## What leaves your network

Each row is off, or sends nothing, unless the setting says otherwise.

| Service | When | Sends | Turn it off |
| --- | --- | --- | --- |
| Your AI provider | You describe a channel | Local Ollama (the default): nothing leaves. A hosted provider (OpenAI, Gemini, Groq, OpenRouter): your intent, plus titles and metadata from your library | Settings → AI: use a local model |
| TMDB | Suggestions, title search, channel icon ideas | Title searches | Leave the TMDB key blank; suggestions then stop working |
| TMDB and your media server | Artwork | Image downloads | `images.remote_fetch_enabled` |
| Filler sources | A source you added in Filler → Sources checks for clips | Requests to that YouTube playlist or Archive.org collection | Disable the source |
| Public reference sources | A clip's era or place is unclear, and an AI model is set up | The clip's public title and search terms to Wikidata, Wikipedia, Archive.org and the Library of Congress | `filler.research.enabled`, or one switch per source |
| Web search | Only if you pick Brave or SearXNG (off by default) | The same public title and search terms, capped per month | `filler.research.web_provider` = none |
| Speech service | Only if you choose a connected one (built-in is the default) | Audio from filler clips, and a short sample when checking a clip's language | `asr.provider` = whisper |
| Vision model | Only if you turn it on (off by default) | A few frames from clips Loomarr can't identify | `filler.vision.enabled` |
| Hugging Face | You open the model list in Settings → AI | A request for popular models; nothing about you | Don't open the list |
| Notifications | Only destinations you add | The alert text, to that service (email, ntfy, browser push) | Remove the destination |

Your browser, not the server, loads Archive.org thumbnails and previews while you browse a
source in Filler → Sources.

A fully local install with a TMDB key sends title searches and artwork requests, and nothing
else. Every setting named here is in the
[settings reference](https://mantonx.github.io/loomarr/reference/settings/).

## Quality measurements stay local

Loomarr can keep local aggregate measurements of how far requested Proposals progressed: finding
candidates, generating and grounding a result, approval, acquisition, and scheduling. This quality
ledger is separate from Prometheus monitoring and is never uploaded.

The ledger does not store title names, your Intent or prompt, locations, account identities,
credentials, viewing history, or raw errors. Stopping playback, deleting a Channel, declining a
Proposal, doing nothing, or leaving a candidate unselected does not mean "dislike" and is never
treated that way. Only the visible **keep**, **less like this**, **never**, and **surprise me**
actions are taste signals.

Detailed transition receipts are kept for 30 days, then reduced to daily aggregates retained for
24 months. An admin can download a sanitized local JSON export of those aggregates and their
versioned evaluation context; raw receipts and internal idempotency keys are never exported.

## Secrets

API keys and passwords you save in Settings are encrypted in the database with the installation
key, and never shown again. A backup leaves that key out on purpose; see
[Back up and restore](../guides/backup.md).

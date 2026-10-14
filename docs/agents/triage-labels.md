# Triage labels

The `triage` skill uses five roles. This table maps them to the repository's GitHub labels.

| Role | GitHub label | Meaning |
| --- | --- | --- |
| Needs triage | `needs-triage` | A maintainer must evaluate the report |
| Needs information | `needs-info` | The reporter must provide missing evidence or decisions |
| Ready for an agent | `ready-for-agent` | The issue is bounded, testable, and safe for an owning agent |
| Ready for a human | `ready-for-human` | The work requires maintainer judgment, credentials, or physical access |
| Will not fix | `wontfix` | The project has deliberately declined the request |

`ready-for-agent` does not bypass `AGENTS.md`: the owning session still checks the roster, claims
shared seams, establishes a red-capable test where appropriate, runs gates, and publishes through
the normal pull-request path.

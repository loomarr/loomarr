package fillervisualsafety

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"net/url"
	"path/filepath"
)

type visualCorpusNominationBoardCase struct {
	Rank          int      `json:"rank"`
	CaseID        string   `json:"caseId"`
	Creator       []string `json:"creator"`
	SubjectTerms  []string `json:"subjectTerms"`
	ObjectURL     string   `json:"objectUrl"`
	ContentSHA256 string   `json:"contentSha256"`
	AssetURL      string   `json:"assetUrl"`
	Width         int      `json:"width"`
	Height        int      `json:"height"`
	ImmutableCSV  []string `json:"immutableCsv"`
}

type visualCorpusNominationBoardData struct {
	WorksheetSHA256 string
	WorksheetJSON   template.JS
	HeaderJSON      template.JS
	CasesJSON       template.JS
}

// RenderVisualCorpusNominationReviewBoard builds a private, non-authorizing
// reviewer aid. The lock operation remains the only consumer of its CSV output.
func RenderVisualCorpusNominationReviewBoard(worksheet VisualCorpusNominationWorksheet, mediaRoot string) ([]byte, error) {
	if validateVisualCorpusNominationWorksheet(worksheet) != nil || !cleanAbsoluteReviewPath(mediaRoot) ||
		validatePrivateReviewDirectory(mediaRoot) != nil {
		return nil, errors.New("visual corpus nomination review board input is invalid")
	}
	cases := make([]visualCorpusNominationBoardCase, len(worksheet.Cases))
	for index, row := range worksheet.Cases {
		assetPath := filepath.Join(mediaRoot, filepath.FromSlash(row.LocalFile))
		assetURL := (&url.URL{Scheme: "file", Path: assetPath}).String()
		cases[index] = visualCorpusNominationBoardCase{
			Rank: row.Rank, CaseID: row.CaseID, Creator: row.Creator, SubjectTerms: row.SubjectTerms,
			ObjectURL: row.ObjectURL, ContentSHA256: row.Asset.SHA256, AssetURL: assetURL,
			Width: row.Width, Height: row.Height,
			ImmutableCSV: ImmutableVisualCorpusNominationCSVRecord(worksheet, row),
		}
	}
	caseBytes, err := json.Marshal(cases)
	if err != nil {
		return nil, errors.New("encode visual corpus nomination review cases")
	}
	worksheetBytes, err := json.Marshal(worksheet.SHA256)
	if err != nil {
		return nil, errors.New("encode visual corpus nomination review worksheet")
	}
	headerBytes, err := json.Marshal(VisualCorpusNominationCSVHeader())
	if err != nil {
		return nil, errors.New("encode visual corpus nomination review header")
	}
	value := visualCorpusNominationBoardData{
		WorksheetSHA256: worksheet.SHA256,
		WorksheetJSON:   template.JS(worksheetBytes), //nolint:gosec // encoding/json escapes the validated digest
		HeaderJSON:      template.JS(headerBytes),    //nolint:gosec // fixed Loomarr-owned field names
		CasesJSON:       template.JS(caseBytes),      //nolint:gosec // encoding/json escapes source-authored text
	}
	var output bytes.Buffer
	if err := visualCorpusNominationBoardTemplate.Execute(&output, value); err != nil {
		return nil, errors.New("render visual corpus nomination review board")
	}
	return output.Bytes(), nil
}

var visualCorpusNominationBoardTemplate = template.Must(template.New("visual-corpus-nomination-review").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Loomarr visual nomination review</title>
<style>
:root { color-scheme: dark; font-family: ui-sans-serif, system-ui, sans-serif; background: #111318; color: #f5f7fa; }
body { margin: 0; min-height: 100vh; display: grid; grid-template-rows: auto 1fr auto; }
header, footer { padding: 14px 20px; background: #1a1e26; border-color: #343a46; }
header { border-bottom: 1px solid #343a46; }
footer { border-top: 1px solid #343a46; display: flex; gap: 10px; align-items: center; flex-wrap: wrap; }
main { display: grid; grid-template-columns: minmax(280px, 1fr) minmax(280px, 420px); gap: 18px; padding: 18px; min-height: 0; }
.image-shell { display: grid; place-items: center; min-height: 55vh; background: #08090c; border-radius: 10px; overflow: hidden; }
img { max-width: 100%; max-height: 72vh; object-fit: contain; }
aside { overflow: auto; }
h1 { margin: 0 0 6px; font-size: 18px; }
h2 { font-size: 14px; margin-top: 18px; }
p { line-height: 1.45; }
.muted { color: #aeb7c7; font-size: 13px; }
.identity { font-family: ui-monospace, monospace; overflow-wrap: anywhere; }
.actions { display: grid; grid-template-columns: repeat(3, minmax(90px, 1fr)); gap: 8px; }
button, .button, input::file-selector-button { border: 1px solid #515b6d; border-radius: 7px; background: #252b36; color: #f5f7fa; padding: 10px 12px; cursor: pointer; text-decoration: none; text-align: center; }
button:hover, button:focus, .button:hover, .button:focus { border-color: #9eb6dc; outline: none; }
button[data-value="positive"] { border-color: #d38888; }
button[data-value="clean"] { border-color: #75b995; }
button[data-value="exclude"] { border-color: #c8a66a; }
button:disabled { cursor: not-allowed; opacity: .45; }
.selected { box-shadow: 0 0 0 2px #f3c969 inset; }
.proposal { color: #80d6aa; }
.hold { color: #f0c979; }
@media (max-width: 760px) { main { grid-template-columns: 1fr; } .image-shell { min-height: 42vh; } }
</style>
</head>
<body>
<header>
  <h1>Loomarr visual nomination review</h1>
  <div id="progress" class="muted"></div>
</header>
<main>
  <section class="image-shell"><img id="candidate" alt="Institutional source work"></section>
  <aside>
    <div id="identity" class="identity"></div>
    <p id="creator"></p>
    <p id="subjects" class="muted"></p>
    <a id="source" class="button" target="_blank" rel="noopener">Open institution record</a>
    <h2>Optional model assistance</h2>
    <input id="assistance" type="file" accept="application/json,.json">
    <p id="assistance-status" class="muted">No assistance loaded. Its suggestions are never decisions.</p>
    <div id="suggestion"></div>
    <button id="exclude-non-proposals" disabled>Exclude all non-proposals</button>
    <h2>Your decision</h2>
    <div class="actions">
      <button data-value="positive">P · Positive</button>
      <button data-value="clean">C · Clean</button>
      <button data-value="exclude">X · Exclude</button>
    </div>
    <p class="muted">Positive and clean always require an individual click or key. Exclusion publishes no candidate or semantic assertion.</p>
  </aside>
</main>
<footer>
  <button id="previous">← Previous</button>
  <button id="next">Next →</button>
  <button id="clear">U · Clear</button>
  <button id="save">S · Download review.csv</button>
  <span class="muted">Worksheet <span class="identity">{{.WorksheetSHA256}}</span></span>
</footer>
<script>
"use strict";
const worksheet = {{.WorksheetJSON}};
const header = {{.HeaderJSON}};
const cases = {{.CasesJSON}};
const decisions = Object.create(null);
const suggestions = Object.create(null);
const storageKey = "loomarr-visual-nomination:" + worksheet;
const assistanceContracts = new Set(["filler-visual-corpus-model-assistance-v1", "filler-visual-corpus-model-assistance-v2"]);
const allowedActions = new Set(["propose_positive_candidate", "exclude_age_risk", "exclude_no_visible_nudity", "hold_cross_batch_creator_overlap", "targeted_human_review"]);
let order = cases.map((_, index) => index);
let cursor = 0;
const image = document.getElementById("candidate");
const identity = document.getElementById("identity");
const creator = document.getElementById("creator");
const subjects = document.getElementById("subjects");
const source = document.getElementById("source");
const progress = document.getElementById("progress");
const suggestion = document.getElementById("suggestion");
const assistanceStatus = document.getElementById("assistance-status");
const excludeNonProposals = document.getElementById("exclude-non-proposals");
const buttons = [...document.querySelectorAll("button[data-value]")];
function current() { return cases[order[cursor]]; }
function counts() {
  const values = Object.values(decisions);
  return {positive: values.filter(value => value === "positive").length,
    clean: values.filter(value => value === "clean").length,
    exclude: values.filter(value => value === "exclude").length};
}
function show() {
  const item = current();
  image.src = item.assetUrl;
  image.width = item.width;
  image.height = item.height;
  identity.textContent = "#" + item.rank + " · " + item.caseId;
  creator.textContent = item.creator.join(", ");
  subjects.textContent = item.subjectTerms.join(" · ");
  source.href = item.objectUrl;
  const totals = counts();
  progress.textContent = "Case " + (cursor + 1) + " of " + cases.length + " · " +
    (totals.positive + totals.clean + totals.exclude) + " decided · " + totals.positive + " positive · " +
    totals.clean + " clean · " + totals.exclude + " excluded";
  const proposed = suggestions[item.caseId];
  if (proposed) {
    const className = proposed.action === "propose_positive_candidate" ? "proposal" : "hold";
    suggestion.className = className;
    suggestion.textContent = proposed.action + " · " + proposed.reason;
  } else {
    suggestion.className = "";
    suggestion.textContent = "";
  }
  for (const button of buttons) button.classList.toggle("selected", decisions[item.caseId] === button.dataset.value);
}
function decide(value) {
  decisions[current().caseId] = value;
  persist();
  if (cursor < cases.length - 1) cursor++;
  show();
}
function persist() {
  try { localStorage.setItem(storageKey, JSON.stringify(decisions)); } catch (_) { /* private-file storage may be unavailable */ }
}
function restore() {
  try {
    const saved = JSON.parse(localStorage.getItem(storageKey) || "{}");
    const valid = new Set(["positive", "clean", "exclude"]);
    for (const item of cases) if (valid.has(saved[item.caseId])) decisions[item.caseId] = saved[item.caseId];
  } catch (_) { /* start empty when storage is unavailable or damaged */ }
}
function move(delta) { cursor = Math.max(0, Math.min(cases.length - 1, cursor + delta)); show(); }
function csvCell(value) {
  const text = String(value);
  return /[",\r\n]/.test(text) ? "\"" + text.replaceAll("\"", "\"\"") + "\"" : text;
}
function save() {
  const rows = [header];
  for (const item of cases) {
    const decision = decisions[item.caseId];
    let mutable = ["", "", "", ""];
    if (decision === "positive") mutable = ["positive_candidate", "historical_art_adult_only", "not_generated", "[\"historical_graphics\"]"];
    else if (decision === "clean") mutable = ["clean_candidate", "no_sensitive_subject_identified", "not_generated", "[\"historical_graphics\"]"];
    else if (decision === "exclude") mutable = ["exclude", "", "", "[]"];
    rows.push(item.immutableCsv.concat(mutable));
  }
  const raw = rows.map(row => row.map(csvCell).join(",")).join("\r\n") + "\r\n";
  const link = document.createElement("a");
  link.href = URL.createObjectURL(new Blob([raw], {type: "text/csv"}));
  link.download = "review.csv";
  link.click();
  URL.revokeObjectURL(link.href);
}
function validateAssistance(document) {
  const first = cases[0].immutableCsv;
  if (!document || document.schemaVersion !== 1 || !assistanceContracts.has(document.contractVersion) ||
      !/^[0-9a-f]{64}$/.test(document.sha256) || document.worksheetSha256 !== worksheet ||
      document.inventorySha256 !== first[1] || document.materializationSha256 !== first[2] ||
      document.candidateModelOutput !== true || document.truthAuthorityCreated !== false ||
      document.trainingAllowed !== false || document.productionUseAllowed !== false ||
      document.ingestionAllowed !== false || document.schedulingAllowed !== false || document.broadcastAllowed !== false ||
      !document.counts || !Array.isArray(document.proposals) || document.proposals.length !== cases.length) throw new Error("manifest authority or worksheet binding is invalid");
  const byCase = new Map(cases.map(item => [item.caseId, item]));
  const seen = new Set();
  const observedCounts = Object.create(null);
  for (const proposed of document.proposals) {
    const item = byCase.get(proposed.caseId);
    if (!item || seen.has(proposed.caseId) || proposed.rank !== item.rank || proposed.sourceSha256 !== item.contentSha256 ||
        !allowedActions.has(proposed.action) || proposed.candidateModelOutput !== true || proposed.truthAuthorityCreated !== false ||
        proposed.trainingAllowed !== false || proposed.productionUseAllowed !== false || typeof proposed.reason !== "string" ||
        proposed.reason.length === 0 || proposed.reason.length > 256) {
      throw new Error("manifest case binding is invalid");
    }
    const positive = proposed.action === "propose_positive_candidate";
    if (positive !== (proposed.proposedNomination === "positive_candidate") ||
        positive !== (proposed.proposedSubjectStatus === "historical_art_adult_only") ||
        positive !== (proposed.proposedGeneratedStatus === "not_generated") ||
        positive !== (Array.isArray(proposed.proposedSlices) && proposed.proposedSlices.length === 1 && proposed.proposedSlices[0] === "historical_graphics")) {
      throw new Error("manifest proposal fields are invalid");
    }
    seen.add(proposed.caseId);
    observedCounts[proposed.action] = (observedCounts[proposed.action] || 0) + 1;
  }
  const countKeys = new Set([...Object.keys(document.counts), ...Object.keys(observedCounts)]);
  for (const key of countKeys) if (!allowedActions.has(key) || document.counts[key] !== (observedCounts[key] || 0)) throw new Error("manifest counts are invalid");
  return document.proposals;
}
async function loadAssistance(event) {
  try {
    const proposals = validateAssistance(JSON.parse(await event.target.files[0].text()));
    for (const key of Object.keys(suggestions)) delete suggestions[key];
    for (const proposed of proposals) suggestions[proposed.caseId] = proposed;
    order = cases.map((_, index) => index).sort((left, right) => {
      const leftProposal = suggestions[cases[left].caseId].action === "propose_positive_candidate" ? 0 : 1;
      const rightProposal = suggestions[cases[right].caseId].action === "propose_positive_candidate" ? 0 : 1;
      return leftProposal - rightProposal || cases[left].rank - cases[right].rank;
    });
    cursor = 0;
    assistanceStatus.textContent = "Bound assistance loaded. Proposed positives are first; suggestions remain non-authoritative.";
    excludeNonProposals.disabled = false;
    show();
  } catch (error) {
    assistanceStatus.textContent = "Assistance rejected: " + error.message;
    excludeNonProposals.disabled = true;
  }
}
function excludeOthers() {
  if (!confirm("Explicitly exclude every model row that is not a positive proposal? This makes no positive or clean decision.")) return;
  for (const item of cases) if (suggestions[item.caseId].action !== "propose_positive_candidate") decisions[item.caseId] = "exclude";
  persist();
  show();
}
for (const button of buttons) button.addEventListener("click", () => decide(button.dataset.value));
document.getElementById("previous").addEventListener("click", () => move(-1));
document.getElementById("next").addEventListener("click", () => move(1));
document.getElementById("clear").addEventListener("click", () => { delete decisions[current().caseId]; persist(); show(); });
document.getElementById("save").addEventListener("click", save);
document.getElementById("assistance").addEventListener("change", loadAssistance);
excludeNonProposals.addEventListener("click", excludeOthers);
document.addEventListener("keydown", event => {
  if (event.target && event.target.tagName === "INPUT") return;
  if (event.key.toLowerCase() === "p") decide("positive");
  else if (event.key.toLowerCase() === "c") decide("clean");
  else if (event.key.toLowerCase() === "x") decide("exclude");
  else if (event.key.toLowerCase() === "u") { delete decisions[current().caseId]; persist(); show(); }
  else if (event.key.toLowerCase() === "s") save();
  else if (event.key === "ArrowLeft") move(-1);
  else if (event.key === "ArrowRight") move(1);
});
restore();
show();
</script>
</body>
</html>
`))

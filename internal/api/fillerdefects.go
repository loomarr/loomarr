package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/loomarr/loomarr/internal/defects"
	"github.com/loomarr/loomarr/internal/filler"
)

// FillerDefectReportDTO is the defect floor's standing on this install (#2034 step 4c).
type FillerDefectReportDTO struct {
	FloorVersion string                       `json:"floorVersion" doc:"The bars every record is judged under once the hourly floor job has caught up"`
	Clips        int                          `json:"clips" doc:"Catalog clips, held ones included"`
	Unmeasured   int                          `json:"unmeasured" doc:"Clips the floor has no readable record for yet"`
	Verdicts     FillerDefectVerdictCountsDTO `json:"verdicts"`
	Labelled     int                          `json:"labelled" doc:"Clips labelled for the era band they are judged in now"`
	Scores       []FillerDefectBandScoreDTO   `json:"scores" doc:"The floor and its judge against the labels, per era band; a band with no labelled clip is absent"`
	Total        int                          `json:"total" doc:"Rows matching the filter"`
	Rows         []FillerDefectRowDTO         `json:"rows" doc:"The requested page, what the floor would retire first"`
}

type FillerDefectVerdictCountsDTO struct {
	Pass      int `json:"pass"`
	Fail      int `json:"fail" doc:"Failing copies still in the catalog: the floor retires one on air at its next pass, and the defects rung refuses one held (#2036)"`
	Undecided int `json:"undecided"`
	Pending   int `json:"pending"`
}

type FillerDefectBandScoreDTO struct {
	Band           string  `json:"band" enum:"tape,sd,hd"`
	Precision      float64 `json:"precision" doc:"Share of the detectors' outright fails labelled fail"`
	Caught         float64 `json:"caught" doc:"Share of clips labelled fail that the detectors failed or sent to the judge"`
	Failed         int     `json:"failed"`
	FalseFails     int     `json:"falseFails"`
	Judged         int     `json:"judged"`
	Missed         int     `json:"missed"`
	BorderlinePass int     `json:"borderlinePass"`
	JudgeRight     int     `json:"judgeRight"`
	JudgeWrong     int     `json:"judgeWrong" doc:"Rulings against the label; an abstention counts here"`
	Total          int     `json:"total"`
}

type FillerDefectRowDTO struct {
	ClipID        string                    `json:"clipId"`
	Name          string                    `json:"name"`
	Era           int                       `json:"era" doc:"The era the record was judged against"`
	Band          string                    `json:"band" enum:"tape,sd,hd"`
	Outcome       string                    `json:"outcome" enum:"pass,borderline,fail,undecided" doc:"The detectors' outcome"`
	Verdict       string                    `json:"verdict" enum:"pass,fail,undecided,pending" doc:"The final standing, a borderline outcome settled by the judge"`
	VerdictReason string                    `json:"verdictReason,omitempty"`
	Findings      []FillerDefectFindingDTO  `json:"findings"`
	Judgement     *FillerDefectJudgementDTO `json:"judgement,omitempty"`
	Label         *FillerDefectLabelDTO     `json:"label,omitempty"`
	MeasuredAt    string                    `json:"measuredAt" format:"date-time"`
}

type FillerDefectFindingDTO struct {
	Check    string  `json:"check" enum:"blocks,blur,dropouts,interlace,windowbox,upscale,clipping"`
	Outcome  string  `json:"outcome" enum:"pass,borderline,fail,undecided"`
	Measured float64 `json:"measured"`
	Bar      float64 `json:"bar" doc:"The borderline bar for a pass or borderline finding, the fail bar for a fail"`
}

type FillerDefectJudgementDTO struct {
	Ruling string `json:"ruling" enum:"pass,fail,abstain,no_judge"`
	Reason string `json:"reason,omitempty"`
	Model  string `json:"model,omitempty"`
}

type FillerDefectLabelDTO struct {
	Label      string `json:"label" enum:"pass,fail"`
	Era        int    `json:"era" doc:"The era the label was given for"`
	Current    bool   `json:"current" doc:"False once the clip's era was corrected into another band: the label is not scored"`
	LabelledAt string `json:"labelledAt" format:"date-time"`
}

type fillerDefectReportInput struct {
	Verdict    string `query:"verdict" enum:"pass,fail,undecided,pending" doc:"Only rows with this verdict"`
	Unlabelled bool   `query:"unlabelled" doc:"Only rows with no current label"`
	Limit      int    `query:"limit" minimum:"1" maximum:"500" default:"100"`
	Offset     int    `query:"offset" minimum:"0"`
}

type fillerDefectReportOutput struct {
	Body FillerDefectReportDTO
}

type labelFillerDefectInput struct {
	ClipID string `path:"clipId" minLength:"1" maxLength:"128"`
	Body   struct {
		Label string `json:"label" enum:"pass,fail" doc:"Would a viewer call this clip damaged against a good copy of its era?"`
	}
}

type clearFillerDefectLabelInput struct {
	ClipID string `path:"clipId" minLength:"1" maxLength:"128"`
}

func (s *Server) registerFillerDefects(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "filler-defect-report", Method: http.MethodGet, Path: "/v1/filler/defects",
		Summary: "Defect floor report",
		Description: "Admin only (#2034). What the defect floor would retire on this install, every measured clip's findings, " +
			"and how the floor and its vision judge score against the maintainer's labels, per era band. " +
			"Labels stay in this install's database; nothing here leaves the machine.",
		Tags: []string{"filler"},
	}, RoleAdmin), s.fillerDefectReport)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "label-filler-defect", Method: http.MethodPut, Path: "/v1/filler/defects/{clipId}/label",
		Summary:     "Label a clip for the defect floor",
		Description: "Admin only (#2034). Labels a measured clip pass or fail against a good copy of the era its record was judged for.",
		Tags:        []string{"filler"},
	}, RoleAdmin), s.labelFillerDefect)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "clear-filler-defect-label", Method: http.MethodDelete, Path: "/v1/filler/defects/{clipId}/label",
		Summary:     "Clear a clip's defect floor label",
		Description: "Admin only (#2034).",
		Tags:        []string{"filler"},
	}, RoleAdmin), s.clearFillerDefectLabel)
}

func (s *Server) fillerDefectReport(ctx context.Context, in *fillerDefectReportInput) (*fillerDefectReportOutput, error) {
	if s.fillerDefects == nil {
		return nil, errFeatureNotConfigured("Defect report unavailable", "The filler defect floor is not configured.")
	}
	r, err := s.fillerDefects.Report(ctx, filler.DefectReportFilter{
		Verdict: defects.Verdict(in.Verdict), Unlabelled: in.Unlabelled, Limit: in.Limit, Offset: in.Offset,
	})
	if err != nil {
		return nil, huma.Error500InternalServerError("read defect report", err)
	}
	return &fillerDefectReportOutput{Body: fillerDefectReportDTO(r)}, nil
}

func (s *Server) labelFillerDefect(ctx context.Context, in *labelFillerDefectInput) (*struct{}, error) {
	return nil, s.setFillerDefectLabel(ctx, in.ClipID, defects.Label(in.Body.Label))
}

func (s *Server) clearFillerDefectLabel(ctx context.Context, in *clearFillerDefectLabelInput) (*struct{}, error) {
	return nil, s.setFillerDefectLabel(ctx, in.ClipID, "")
}

func (s *Server) setFillerDefectLabel(ctx context.Context, clipID string, label defects.Label) error {
	if s.fillerDefects == nil {
		return errFeatureNotConfigured("Defect report unavailable", "The filler defect floor is not configured.")
	}
	err := s.fillerDefects.Label(ctx, clipID, label)
	if errors.Is(err, filler.ErrDefectNotMeasured) {
		return huma.Error404NotFound("The defect floor has not measured this clip yet")
	}
	if err != nil {
		return huma.Error500InternalServerError("label defect", err)
	}
	return nil
}

func fillerDefectReportDTO(r filler.DefectReport) FillerDefectReportDTO {
	out := FillerDefectReportDTO{
		FloorVersion: r.FloorVersion, Clips: r.Clips, Unmeasured: r.Unmeasured, Labelled: r.Labelled, Total: r.Total,
		Verdicts: FillerDefectVerdictCountsDTO{
			Pass: r.Verdicts[defects.VerdictPass], Fail: r.Verdicts[defects.VerdictFail],
			Undecided: r.Verdicts[defects.VerdictUndecided], Pending: r.Verdicts[defects.VerdictPending],
		},
		Scores: make([]FillerDefectBandScoreDTO, 0, len(r.Scores)),
		Rows:   make([]FillerDefectRowDTO, 0, len(r.Rows)),
	}
	for _, s := range r.Scores {
		out.Scores = append(out.Scores, FillerDefectBandScoreDTO{
			Band: string(s.Band), Precision: s.Precision(), Caught: s.Caught(),
			Failed: s.Failed, FalseFails: s.FalseFails, Judged: s.Judged, Missed: s.Missed,
			BorderlinePass: s.BorderlinePass, JudgeRight: s.JudgeRight, JudgeWrong: s.JudgeWrong, Total: s.Total,
		})
	}
	for _, row := range r.Rows {
		rec := row.Record
		dto := FillerDefectRowDTO{
			ClipID: row.Clip.ID(), Name: row.Clip.Name, Era: rec.Era, Band: string(rec.Assessment.Band),
			Outcome: string(rec.Assessment.Outcome), Verdict: string(rec.Verdict), VerdictReason: rec.VerdictReason,
			Findings:   make([]FillerDefectFindingDTO, 0, len(rec.Assessment.Findings)),
			MeasuredAt: rec.MeasuredAt.UTC().Format(time.RFC3339),
		}
		for _, f := range rec.Assessment.Findings {
			dto.Findings = append(dto.Findings, FillerDefectFindingDTO{
				Check: string(f.Check), Outcome: string(f.Outcome), Measured: f.Measured, Bar: f.Bar,
			})
		}
		if j := rec.Judgement; j != nil {
			dto.Judgement = &FillerDefectJudgementDTO{Ruling: string(j.Ruling), Reason: j.Reason, Model: j.Model}
		}
		if l := row.Label; l != nil {
			dto.Label = &FillerDefectLabelDTO{Label: string(l.Label), Era: l.Era, Current: row.LabelCurrent,
				LabelledAt: l.LabelledAt.UTC().Format(time.RFC3339)}
		}
		out.Rows = append(out.Rows, dto)
	}
	return out
}

package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/loomarr/loomarr/internal/holidayvocab"
)

// registerHolidays mounts the holiday definitions (#2030 decision 32). Read-only until the holidays
// editor ships with its create, edit and retire endpoints.
func (s *Server) registerHolidays(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "list-holidays", Method: http.MethodGet, Path: "/v1/holidays",
		Summary:     "The holiday definitions and this year's dates",
		Description: "Every holiday definition in display order — its words, each country's date rules, its broadcast air window and, for a group, its members — with this year's season for the household's country (`season`, absent when that country does not keep it). `calendarCountry` is the country whose calendar applies: the household's own when some holiday names it, else the US. Read-only; any authenticated user.",
		Tags:        []string{"channels"},
	}, RoleMember), s.listHolidays)
}

type holidaysOutput struct {
	Body HolidaysDTO
}

// HolidaysDTO is the holiday definitions as the household's calendar reads them.
type HolidaysDTO struct {
	// Country is the household's country setting (filler.home_country), as set.
	Country string `json:"country"`
	// CalendarCountry is the country whose calendar applies (see the operation description).
	CalendarCountry string       `json:"calendarCountry"`
	Holidays        []HolidayDTO `json:"holidays"`
}

// HolidayDTO is one holiday definition and its season this year.
type HolidayDTO struct {
	ID                     string                 `json:"id"`
	Label                  string                 `json:"label"`
	Words                  holidayvocab.Words     `json:"words"`
	Kept                   []holidayvocab.Keeping `json:"kept"`
	AirWindow              *holidayvocab.Rule     `json:"airWindow,omitempty"`
	Members                []string               `json:"members"`
	NameNeedsCorroboration bool                   `json:"nameNeedsCorroboration"`
	Season                 *HolidaySeasonDTO      `json:"season,omitempty"`
}

// HolidaySeasonDTO is a holiday's season in one year, as calendar dates (YYYY-MM-DD), inclusive.
type HolidaySeasonDTO struct {
	Start string `json:"start" format:"date"`
	End   string `json:"end" format:"date"`
	Day   string `json:"day" format:"date" doc:"The holiday's own date."`
}

func (s *Server) listHolidays(context.Context, *struct{}) (*holidaysOutput, error) {
	cal := s.holidayCalendar()
	vocab := cal.Vocab()
	now := s.clock()
	if cal.Zone != nil {
		now = now.In(cal.Zone)
	}
	year := now.Year()
	seasons := map[string]*HolidaySeasonDTO{}
	for _, o := range vocab.Calendar(cal.Country) {
		start, end := o.Window(year)
		seasons[o.ID] = &HolidaySeasonDTO{
			Start: start.Format(time.DateOnly), End: end.Format(time.DateOnly), Day: o.Day(year).Format(time.DateOnly),
		}
	}
	out := HolidaysDTO{Country: cal.Country, CalendarCountry: vocab.CalendarCountry(cal.Country)}
	for _, h := range vocab.Holidays() {
		out.Holidays = append(out.Holidays, HolidayDTO{
			ID: h.ID, Label: h.Label, Words: h.Words, Kept: h.Kept, AirWindow: h.AirWindow,
			Members: h.Members, NameNeedsCorroboration: h.NameNeedsCorroboration, Season: seasons[h.ID],
		})
	}
	return &holidaysOutput{Body: out}, nil
}

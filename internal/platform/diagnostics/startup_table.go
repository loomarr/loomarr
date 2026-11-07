package diagnostics

import (
	"fmt"
	"strings"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

// StartupTableOptions controls only presentation. It never changes or persists report state.
type StartupTableOptions struct {
	Width int
	Color bool
}

// RenderStartupTable renders the useful operator columns from one caller-owned report snapshot.
func RenderStartupTable(report StartupReport, opts StartupTableOptions) string {
	width := opts.Width
	if width <= 0 {
		width = 100
	}
	w := table.NewWriter()
	w.SetStyle(table.StyleRounded)
	w.AppendHeader(table.Row{"Status", "Check", "Duration", "Detail"})
	w.SetColumnConfigs([]table.ColumnConfig{
		{Name: "Status", WidthMax: 10},
		{Name: "Check", WidthMax: min(28, max(14, width/4))},
		{Name: "Duration", WidthMax: 10, Align: text.AlignRight},
		{Name: "Detail", WidthMax: max(16, width-57)},
	})
	for _, check := range report.Checks {
		status := strings.ToUpper(string(check.Status))
		if opts.Color {
			status = startupStatusANSI(check.Status, status)
		}
		duration := "—"
		if check.EndedAt != 0 {
			duration = (time.Duration(check.DurationMillis) * time.Millisecond).Round(time.Millisecond).String()
		}
		detail := check.Detail
		if detail == "" && check.RemediationRoute != "" {
			detail = "Open " + check.RemediationRoute
		} else if check.RemediationRoute != "" {
			detail += " · Open " + check.RemediationRoute
		}
		w.AppendRow(table.Row{status, check.Label, duration, detail})
	}
	headline := fmt.Sprintf("Loomarr %s · generation %d · %s · %s", report.Version, report.Generation,
		strings.ToUpper(string(report.State)), formatStartupDuration(report.DurationMillis))
	return text.WrapSoft(headline, width) + "\n" + w.Render()
}

func startupStatusANSI(status StartupCheckStatus, value string) string {
	code := "36"
	switch status {
	case StartupPassed:
		code = "32"
	case StartupWarning:
		code = "33"
	case StartupFailed:
		code = "31"
	case StartupSkipped:
		code = "90"
	}
	return "\x1b[" + code + "m" + value + "\x1b[0m"
}

func formatStartupDuration(ms int64) string {
	if ms <= 0 {
		return "in progress"
	}
	return fmt.Sprintf("started in %s", (time.Duration(ms) * time.Millisecond).Round(time.Millisecond))
}

package recommend

import (
	"fmt"
	"strings"
)

func HumanDevelopmentSummary(card DevelopmentScorecard) string {
	status := "INCOMPLETE"
	if card.Completed {
		status = "COMPLETE"
	}
	var summary strings.Builder
	fmt.Fprintf(&summary, "# Channel recommendation development diagnostic: %s\n\n", status)
	fmt.Fprintf(&summary, "Corpus `%s` (`%s`), protocol `%s`, provider `%s`, model `%s`, profile `%s`.\n\n",
		card.CorpusVersion, card.CorpusSHA256, card.Protocol, card.Provider, card.Model, card.Profile)
	fmt.Fprintf(&summary, "Resources: %d calls, %d tokens, $%s. This development result cannot certify a model.\n",
		card.Resources.Calls, card.Resources.Tokens, formatNanoUSD(card.Resources.SpendNanoUSD))
	if !card.Resources.AccountingComplete {
		summary.WriteString("Resource accounting is incomplete; the displayed total is not a spend claim.\n")
	}
	if card.StopReason != "" {
		fmt.Fprintf(&summary, "Stopped: `%s`.\n", card.StopReason)
	}
	return summary.String()
}

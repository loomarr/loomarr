package app

import (
	"log/slog"
	"time"

	guideapi "github.com/loomarr/loomarr/internal/api/guide"
	"github.com/loomarr/loomarr/internal/channels/approval"
	"github.com/loomarr/loomarr/internal/library/mediaserver"
	"github.com/loomarr/loomarr/internal/library/titlesearch"
	"github.com/loomarr/loomarr/internal/platform/activity"
	"github.com/loomarr/loomarr/internal/platform/holidayvocab"
	"github.com/loomarr/loomarr/internal/playout/resolve"
	"github.com/loomarr/loomarr/internal/proposals/accept"
	"github.com/loomarr/loomarr/internal/proposals/quality"
	"github.com/loomarr/loomarr/internal/proposals/suggest"
)

type approvalBuild struct {
	binder          *approval.Binder
	approver        *accept.Approver
	decisionQuality *quality.ProposalDecisionRecorder
}

func buildApproval(
	st approvalStores,
	channelService guideapi.ChannelService,
	playoutResolver *resolve.Resolver,
	activityRecorder *activity.Recorder,
	channelNumbers approval.NumberSource,
	notifier suggest.ProposalNotifier,
	libraryClient *mediaserver.Client,
	holidays holidayvocab.Source,
	log *slog.Logger,
) approvalBuild {
	if st.binder == nil {
		return approvalBuild{}
	}
	var reconciler approval.Reconciler
	if channelService != nil {
		reconciler = channelService
	}
	var codec approval.CodecComputer
	if playoutResolver != nil {
		codec = playoutResolver
	}
	channelBinder := approval.New(st.binder, reconciler, codec, log)
	if activityRecorder != nil {
		channelBinder = channelBinder.WithActivity(activityRecorder)
	}
	if channelNumbers != nil {
		channelBinder = channelBinder.WithChannelNumbers(channelNumbers)
	}
	decisionQuality := quality.NewProposalDecisionRecorder(st.observations, log)
	approver := accept.NewApprover(st.approver, channelBinder, time.Now).
		WithProposalNotifier(notifier).
		WithDecisionQuality(decisionQuality).
		WithHolidays(holidays)
	approver = approver.WithApprovalAdditionResolver(accept.NewAdditionResolver(func() titlesearch.LibraryPresence {
		return titlesearch.LibraryOwnership{Titles: st.library}
	}))
	return approvalBuild{
		binder:          channelBinder,
		approver:        approver,
		decisionQuality: decisionQuality,
	}
}

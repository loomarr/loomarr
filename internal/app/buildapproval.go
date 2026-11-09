package app

import (
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/channels/approval"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/platform/activity"
	"github.com/loomarr/loomarr/internal/platform/holidayvocab"
	"github.com/loomarr/loomarr/internal/platform/store"
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
	st store.Store,
	channelService api.ChannelService,
	playoutResolver *playoutResolver,
	activityRecorder *activity.Recorder,
	channelNumbers approval.NumberSource,
	notifier suggest.ProposalNotifier,
	libraryClient *library.Client,
	holidays holidayvocab.Source,
	log *slog.Logger,
) approvalBuild {
	if st == nil {
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
	channelBinder := approval.New(st, reconciler, codec, log)
	if activityRecorder != nil {
		channelBinder = channelBinder.WithActivity(activityRecorder)
	}
	if channelNumbers != nil {
		channelBinder = channelBinder.WithChannelNumbers(channelNumbers)
	}
	decisionQuality := quality.NewProposalDecisionRecorder(st, log)
	approver := accept.NewApprover(st, channelBinder, time.Now).
		WithProposalNotifier(notifier).
		WithDecisionQuality(decisionQuality).
		WithHolidays(holidays)
	approver = approver.WithApprovalAdditionResolver(approvalAdditionAdapter{presence: func() catalog.LibraryPresence {
		return catalog.LibraryOwnership{Titles: st}
	}})
	return approvalBuild{
		binder:          channelBinder,
		approver:        approver,
		decisionQuality: decisionQuality,
	}
}

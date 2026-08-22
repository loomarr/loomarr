package app

import (
	"log/slog"
	"time"

	"github.com/mantonx/loomarr/internal/activity"
	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/binder"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
)

type approvalBuild struct {
	binder   *binder.Binder
	approver *suggest.Approver
}

func buildApproval(
	st store.Store,
	channelService api.ChannelService,
	playoutResolver *playoutResolver,
	activityRecorder *activity.Recorder,
	channelNumbers binder.NumberSource,
	log *slog.Logger,
) approvalBuild {
	if st == nil {
		return approvalBuild{}
	}
	var reconciler binder.Reconciler
	if channelService != nil {
		reconciler = channelService
	}
	var codec binder.CodecComputer
	if playoutResolver != nil {
		codec = playoutResolver
	}
	channelBinder := binder.New(st, reconciler, codec, log)
	if activityRecorder != nil {
		channelBinder = channelBinder.WithActivity(activityRecorder)
	}
	if channelNumbers != nil {
		channelBinder = channelBinder.WithChannelNumbers(channelNumbers)
	}
	return approvalBuild{
		binder: channelBinder, approver: suggest.NewApprover(st, channelBinder, time.Now),
	}
}

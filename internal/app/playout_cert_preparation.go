package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"slices"
	"time"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/media"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/prepared"
	"github.com/loomarr/loomarr/internal/store"
)

type certificationSource struct {
	identity prepared.Source
	path     string
	size     int64
	modified time.Time
}

// certificationPreparation supplies only the disposable schedule and frozen
// source cohort. The production runtime resolver owns all readiness decisions.
type certificationPreparation struct {
	store.Store
	schedule syntheticProgrammeSchedule
	channels map[string]bool
	sources  map[string]certificationSource
}

func newCertificationPreparation(ctx context.Context, st store.Store, sources certificationSources, schedule syntheticProgrammeSchedule, indexes []int, config PlayoutCertificationConfig) (*certificationPreparation, error) {
	a := &certificationPreparation{Store: st, schedule: schedule, channels: make(map[string]bool), sources: make(map[string]certificationSource)}
	a.schedule.items = make(map[string][2]string)
	byPath := make(map[string]certificationSource)
	for _, index := range indexes {
		a.channels[config.Channels[index].ID] = true
	}
	for _, channel := range config.Channels {
		var items [2]string
		for variant, path := range sources.paths[channel.ID] {
			source, ok := byPath[path]
			if !ok {
				file, err := os.Open(path)
				if err != nil {
					return nil, errors.New("certification source unavailable")
				}
				info, statErr := file.Stat()
				hash := sha256.New()
				readErr := hashCertificationSource(ctx, file, hash)
				closeErr := file.Close()
				if statErr != nil || readErr != nil || closeErr != nil || !info.Mode().IsRegular() {
					return nil, errors.New("certification source evidence unavailable")
				}
				digest := hex.EncodeToString(hash.Sum(nil))
				source = certificationSource{identity: prepared.Source{ItemID: "cert-" + digest, SourceID: digest, Revision: digest}, path: path, size: info.Size(), modified: info.ModTime()}
				byPath[path] = source
				a.sources[source.identity.ItemID] = source
			}
			items[variant] = source.identity.ItemID
		}
		a.schedule.items[channel.ID] = items
	}
	return a, nil
}

func hashCertificationSource(ctx context.Context, source io.Reader, hash io.Writer) error {
	buffer := make([]byte, 64<<10)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := source.Read(buffer)
		if n > 0 {
			if _, writeErr := hash.Write(buffer[:n]); writeErr != nil {
				return writeErr
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (a *certificationPreparation) ListChannels(ctx context.Context) ([]store.Channel, error) {
	channels, err := a.Store.ListChannels(ctx)
	return slices.DeleteFunc(channels, func(ch store.Channel) bool { return !a.channels[ch.ID] }), err
}

func (a *certificationPreparation) ScheduledBroadcasts(ctx context.Context, channel string, from, to time.Time) ([]playout.Broadcast, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !a.channels[channel] || !to.After(from) {
		return nil, nil
	}
	var broadcasts []playout.Broadcast
	for ordinal := a.schedule.ordinal(from); ; ordinal++ {
		broadcast := a.schedule.broadcast(channel, ordinal)
		if !broadcast.Start.Before(to) {
			break
		}
		if broadcast.Stop.After(from) {
			broadcasts = append(broadcasts, broadcast)
		}
		if len(broadcasts) > 12_000 {
			return nil, errors.New("certification schedule window exceeds bounds")
		}
	}
	return broadcasts, nil
}

func (*certificationPreparation) AudioTrackFor(context.Context, string, string, string) int {
	return 0
}
func (*certificationPreparation) AudioTrackFromInventory(context.Context, string, string) (int, bool) {
	return 0, true
}

func (a *certificationPreparation) ResolvePreparedSourceFromInventory(ctx context.Context, item string, paths library.PathMap) (prepared.Source, string, bool) {
	return a.ResolvePreparedSource(ctx, item, paths)
}

func (a *certificationPreparation) ResolvePreparedSource(ctx context.Context, item string, _ library.PathMap) (prepared.Source, string, bool) {
	source, ok := a.sources[item]
	if !ok || !a.PreparedSourceCurrent(ctx, source.identity) {
		return prepared.Source{}, "", false
	}
	return source.identity, source.path, true
}

func (a *certificationPreparation) PreparedSourceCurrent(ctx context.Context, identity prepared.Source) bool {
	source, ok := a.sources[identity.ItemID]
	if !ok || identity != source.identity || ctx.Err() != nil {
		return false
	}
	info, err := os.Stat(source.path)
	return err == nil && info.Mode().IsRegular() && info.Size() == source.size && info.ModTime().Equal(source.modified)
}

func (a *certificationPreparation) OpenInput(ctx context.Context, source prepared.Source) (prepared.Input, error) {
	if !a.PreparedSourceCurrent(ctx, source) {
		return prepared.Input{}, prepared.ErrSourceChanged
	}
	return prepared.LocalInput(a.sources[source.ItemID].path), nil
}

func (t *PlayoutCertificationTarget) prepareDeclared(ctx context.Context, config PlayoutCertificationConfig, sources certificationSources, library *prepared.Library, packager prepared.Packager, schedule syntheticProgrammeSchedule, indexes []int) (playout.PreparedResolver, syntheticProgrammeSchedule, error) {
	adapter, err := newCertificationPreparation(ctx, t.store, sources, schedule, indexes, config)
	if err != nil {
		return nil, schedule, err
	}
	readiness, err := prepared.OpenReadiness(library)
	if err != nil {
		return nil, schedule, err
	}
	preparer := prepared.NewPreparer(prepared.PreparerDependencies{Library: library, Packager: packager, Access: adapter})
	runtime := newPreparedRuntimeResolver(preparedRuntimeDependencies{
		Channels: adapter, Timeline: adapter, Sources: adapter, Lookup: preparer, Readiness: readiness,
		Now: time.Now, Policy: func() string { return "certification-" + string(config.QualityTier) },
		Rendition: config.rendition,
	})
	t.encodePool = media.NewEncodePool(func() int {
		if playout.IsSoftwareEncoder(t.encoder) {
			return 0
		}
		return config.Capacity
	})
	t.preparation = prepared.NewPlanner(prepared.PlannerDependencies{
		Resolver: runtime, Preparation: preparer, Pool: t.encodePool, Retainer: library,
		BudgetBytes: func() int64 { return preparedBudgetBytes(512) }, Now: time.Now,
	})
	for pass := 0; pass < 32; pass++ {
		if err := t.preparation.Run(ctx); err != nil {
			return nil, schedule, err
		}
		status := t.preparation.Status()
		if !status.LastRunAt.IsZero() && status.Readiness.ReadyChannels == len(indexes) && status.Readiness.MissingBindings == 0 {
			break
		}
		if pass == 31 || ctx.Err() != nil || playout.IsSoftwareEncoder(t.encoder) || config.Capacity < 2 {
			return nil, schedule, errors.New("declared preparation did not converge")
		}
	}
	truthCache := make(map[prepared.Specification]map[string]syntheticAssetTruth)
	for _, index := range indexes {
		channel := config.Channels[index].ID
		var truths [2]map[string]syntheticAssetTruth
		for variant, item := range adapter.schedule.items[channel] {
			spec, ready, err := preparer.Lookup(prepared.Request{Source: adapter.sources[item].identity, Rendition: config.rendition()})
			if err != nil || !ready {
				return nil, schedule, errors.New("prepared publication evidence unavailable")
			}
			if truth, ok := truthCache[spec]; ok {
				truths[variant] = truth
				continue
			}
			publication, ready, err := library.Lookup(spec)
			if err != nil || !ready {
				return nil, schedule, errors.New("prepared publication evidence unavailable")
			}
			truths[variant], err = readCertificationPublicationTruth(ctx, config.FFmpeg, publication)
			if err != nil {
				return nil, schedule, err
			}
			truthCache[spec] = truths[variant]
		}
		t.programmeEvidence.assets[channel] = truths
	}
	return runtime, adapter.schedule, nil
}

func (t *PlayoutCertificationTarget) startPreparation(ctx context.Context) {
	if t.preparation == nil {
		return
	}
	ctx, t.preparationCancel = context.WithCancel(ctx)
	t.preparationDone = make(chan struct{})
	go func() {
		defer close(t.preparationDone)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = t.preparation.Run(ctx) // The ordinary status projection retains failures.
			}
		}
	}()
}

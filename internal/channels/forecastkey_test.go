package channels_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout/tunarr/tunarrtest"
	"github.com/loomarr/loomarr/internal/testkit"
)

// forecastFieldClass is how a channel field reaches the forecast.
type forecastFieldClass int

const (
	keyed      forecastFieldClass = iota // read by the forecast and hashed by ForecastKey
	derived                              // set during assembly from keyed fields, settings or engine state; never stored
	notAnInput                           // never read by an arrangement
)

// forecastFields classifies every field of channelstore.Channel, the embedded schedule.Channel
// included. A new field fails TestForecastKeyClassifiesEveryChannelField until it is classified
// here, and a keyed one must move the key.
var forecastFields = map[string]forecastFieldClass{
	// channelstore.Channel
	"Revision": notAnInput, "IntentJSON": notAnInput, "RequestedBy": notAnInput,
	"Lineup": keyed, "Desired": keyed, "Policy": keyed, "Findings": notAnInput, "BroadcastCodec": notAnInput,
	"PlayoutAnchor": keyed, "DealtFrom": notAnInput, "ReconcileDeadline": notAnInput, "CreatedAt": keyed,
	// schedule.Channel
	"ID": keyed, "IntentRef": notAnInput, "FromRequest": notAnInput, "Name": notAnInput,
	"Number": notAnInput, "Group": notAnInput, "Logo": notAnInput, "Strategy": keyed,
	"BreaksPerHour": derived, "BreakDurationMs": derived, "NaturalBreaks": derived,
	"PinnedCuts": derived, "LastAired": derived, "DefaultWindow": derived, "WindowZone": derived,
	"HolidayCountry": derived, "HolidayDefinitions": derived, "WindowOpened": derived,
	"DealOrigin": derived, "TunarrID": notAnInput, "Status": notAnInput, "Shuffle": keyed,
	"UpdatedAt": notAnInput,
}

func forecastKeyEngine(breakLength *time.Duration) *channels.Engine {
	return channels.New(nil, nil, mapAvail{}, nil, channels.Config{
		ResolveBreakDuration: func() time.Duration { return *breakLength },
		ResolvePlayoutBackendContext: func(context.Context) (string, error) {
			return schedule.PlayoutBackendInternal, nil
		},
	}, func() time.Time { return equivalenceStart }, testkit.Logger())
}

func TestForecastKeyClassifiesEveryChannelField(t *testing.T) {
	length := time.Minute
	e := forecastKeyEngine(&length)
	ctx := context.Background()
	base := channelstore.Channel{Lineup: []schedule.LineupEntry{entry("movie:tmdb:90000001", "A")}}
	base.ID = "c1"
	want, ok := e.ForecastKey(ctx, base, equivalenceStart)
	if !ok {
		t.Fatal("no key for a plain channel")
	}

	var walk func(v reflect.Value, path string)
	walk = func(v reflect.Value, path string) {
		for i := range v.NumField() {
			f := v.Type().Field(i)
			if f.Anonymous {
				walk(v.Field(i), path)
				continue
			}
			class, known := forecastFields[f.Name]
			if !known {
				t.Errorf("%s%s is not classified in forecastFields: decide whether the forecast reads it", path, f.Name)
				continue
			}
			if class != keyed {
				continue
			}
			ch := base
			ch.Lineup = append([]schedule.LineupEntry(nil), base.Lineup...)
			field := reflect.ValueOf(&ch).Elem().FieldByName(f.Name)
			if !perturb(field) {
				t.Errorf("%s is keyed but the test cannot change a %s; add a case to perturb", f.Name, field.Type())
				continue
			}
			if got, _ := e.ForecastKey(ctx, ch, equivalenceStart); got == want {
				t.Errorf("changing %s leaves ForecastKey unchanged: a cached forecast would outlive the change", f.Name)
			}
		}
	}
	walk(reflect.ValueOf(base), "")
}

// perturb changes v to a different value of its type, reporting false for a kind it cannot change.
func perturb(v reflect.Value) bool {
	if v.Type() == reflect.TypeFor[time.Time]() {
		v.Set(reflect.ValueOf(v.Interface().(time.Time).Add(time.Hour)))
		return true
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(v.String() + "-changed")
	case reflect.Int, reflect.Int64, reflect.Int32:
		v.SetInt(v.Int() + 1)
	case reflect.Bool:
		v.SetBool(!v.Bool())
	case reflect.Slice:
		v.Set(reflect.Append(v, reflect.Zero(v.Type().Elem())))
	case reflect.Pointer:
		if !v.IsNil() {
			return perturb(v.Elem())
		}
		v.Set(reflect.New(v.Type().Elem()))
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() && perturb(v.Field(i)) {
				return true
			}
		}
		return false
	default:
		return false
	}
	return true
}

// Settings the forecast reads move the key at once rather than after the guide cache's TTL.
func TestForecastKeyFollowsTheSettings(t *testing.T) {
	length := time.Minute
	e := forecastKeyEngine(&length)
	ch := channelstore.Channel{Lineup: []schedule.LineupEntry{entry("movie:tmdb:90000001", "A")}}
	ch.ID = "c1"
	before, _ := e.ForecastKey(context.Background(), ch, equivalenceStart)
	length = 2 * time.Minute
	if after, _ := e.ForecastKey(context.Background(), ch, equivalenceStart); after == before {
		t.Fatal("a changed break length leaves ForecastKey unchanged")
	}
}

// A reconcile that changes nothing about the deal keeps the key, so the guide cache keeps its
// entry across sweeps even though every save moves the row's revision and deadlines.
func TestForecastKeyReconcileWithNoChangeKeepsTheEntry(t *testing.T) {
	for _, backend := range []string{schedule.PlayoutBackendTunarr, schedule.PlayoutBackendInternal} {
		t.Run(backend, func(t *testing.T) { reconcileKeepsForecastKey(t, backend) })
	}
}

func reconcileKeepsForecastKey(t *testing.T, backend string) {
	ctx := context.Background()
	st := newStore(t)
	now := equivalenceStart
	e := channels.New(st, tunarrtest.NewTunarr(), mapAvail{
		provision.Key("movie:tmdb:90000001"): "lib-1", provision.Key("movie:tmdb:90000002"): "lib-2",
	}, nil, channels.Config{
		ReconcileTTL:                 time.Minute,
		ResolvePlayoutBackendContext: func(context.Context) (string, error) { return backend, nil },
	}, func() time.Time { return now }, testkit.Logger()).WithPods(&fakePods{ids: []string{"clip-a"}})
	seedChannel(t, st, "c1", 5, entry("movie:tmdb:90000001", "A"), entry("movie:tmdb:90000002", "B"))
	if err := e.Reconcile(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	first, err := st.GetChannel(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(10 * time.Minute)
	if err := e.Reconcile(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	second, err := st.GetChannel(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision == second.Revision && first.ReconcileDeadline.Equal(second.ReconcileDeadline) {
		t.Fatal("the second reconcile saved nothing, so the test proves nothing")
	}
	k1, _ := e.ForecastKey(ctx, first, equivalenceStart)
	k2, _ := e.ForecastKey(ctx, second, equivalenceStart)
	if k1 != k2 {
		t.Fatal("a reconcile that changed nothing moved ForecastKey: every sweep would empty the guide cache")
	}
}

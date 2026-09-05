// Command prototype-playout-hotset answers one throwaway design question:
// does the current session manager bound grace-idle video-copy sessions while a viewer rapidly
// surfs a large lineup? It drives the production Manager with fake encoder pipes and prints the
// complete relevant state after every action. This command belongs only on the prototype branch.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
)

const (
	bold  = "\x1b[1m"
	dim   = "\x1b[2m"
	reset = "\x1b[0m"
)

type encoder struct {
	writer *os.File
	stop   sync.Once
}

type prototype struct {
	manager *playout.Manager

	mu       sync.Mutex
	encoders map[string]*encoder
	spawned  int
	stopped  int

	current string
	detach  func()
	next    int
	lastErr error
}

func newPrototype() *prototype {
	p := &prototype{encoders: make(map[string]*encoder)}
	spawn := func(ctx context.Context, channelID string, _ playout.EncodePlan) (*playout.Process, error) {
		reader, writer, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		enc := &encoder{writer: writer}
		p.mu.Lock()
		p.encoders[channelID] = enc
		p.spawned++
		p.mu.Unlock()
		go func() {
			<-ctx.Done()
			enc.stop.Do(func() {
				_ = writer.Close()
				p.mu.Lock()
				p.stopped++
				p.mu.Unlock()
			})
		}()
		return &playout.Process{Stdout: reader}, nil
	}
	// One transcode slot makes the distinction visible: every cold session reserves it, then
	// ReportProgram marks the warmed source as video-copy and releases it. A total hot-set bound
	// would still constrain those zero-transcode-cost sessions.
	p.manager = playout.NewManager(spawn, func() int { return 1 }, 30*time.Second, nil)
	return p
}

func (p *prototype) close() {
	if p.detach != nil {
		p.detach()
	}
	p.manager.Stop()
}

func (p *prototype) surf() {
	if p.detach != nil {
		p.detach()
		p.detach = nil
	}
	p.next++
	channelID := fmt.Sprintf("channel-%03d", p.next)
	viewer, detach, err := p.manager.Attach(context.Background(), channelID, playout.PlanFull)
	if err != nil {
		p.lastErr = err
		return
	}
	p.mu.Lock()
	enc := p.encoders[channelID]
	p.mu.Unlock()
	if enc == nil {
		detach()
		p.lastErr = fmt.Errorf("missing fake encoder for %s", channelID)
		return
	}
	if _, err := enc.writer.Write([]byte("transport")); err != nil {
		detach()
		p.lastErr = err
		return
	}
	select {
	case <-viewer:
	case <-time.After(time.Second):
		detach()
		p.lastErr = fmt.Errorf("%s produced no transport", channelID)
		return
	}
	p.manager.ReportProgram(channelID, playout.PlanFull, playout.EncoderSoftware, false, playout.Progress{})
	p.current = channelID
	p.detach = detach
	p.lastErr = nil
}

func (p *prototype) burst(count int) {
	for range count {
		p.surf()
		if p.lastErr != nil {
			return
		}
	}
}

func (p *prototype) render(clear bool) {
	if clear {
		fmt.Print("\x1b[2J\x1b[H")
	}
	stats := p.manager.Stats(time.Now())
	active, idle := 0, 0
	idleChannels := make([]string, 0, len(stats))
	for _, stat := range stats {
		if stat.Viewers > 0 {
			active++
		} else {
			idle++
			idleChannels = append(idleChannels, stat.ChannelID)
		}
	}
	sort.Strings(idleChannels)
	p.mu.Lock()
	spawned, stopped := p.spawned, p.stopped
	p.mu.Unlock()

	fmt.Printf("%sWarm-session hot-set prototype%s\n", bold, reset)
	fmt.Printf("%sQuestion:%s are grace-idle video-copy sessions globally bounded?\n\n", dim, reset)
	fmt.Printf("%sChannels visited:%s %d\n", bold, reset, p.next)
	fmt.Printf("%sCurrent channel:%s %s\n", bold, reset, valueOrDash(p.current))
	fmt.Printf("%sLive sessions:%s %d\n", bold, reset, len(stats))
	fmt.Printf("%sViewer-active:%s %d\n", bold, reset, active)
	fmt.Printf("%sGrace-idle:%s %d\n", bold, reset, idle)
	fmt.Printf("%sEncoders spawned/stopped:%s %d / %d\n", bold, reset, spawned, stopped)
	fmt.Printf("%sTranscode budget:%s 1 (all warmed programmes reported as video-copy)\n", bold, reset)
	if p.lastErr != nil {
		fmt.Printf("%sLast error:%s %v\n", bold, reset, p.lastErr)
	}
	if len(idleChannels) > 0 {
		shown := idleChannels
		if len(shown) > 8 {
			shown = shown[len(shown)-8:]
		}
		fmt.Printf("%sNewest idle channels:%s %s\n", bold, reset, strings.Join(shown, ", "))
	}
	fmt.Printf("\n%ss%s surf next  %sb%s burst 50  %sr%s render  %sq%s quit\n", bold, reset, bold, reset, bold, reset, bold, reset)
}

func valueOrDash(value string) string {
	if value == "" {
		return "—"
	}
	return value
}

func main() {
	burst := flag.Int("burst", 0, "run this many surf actions, print the final state, and exit")
	flag.Parse()
	p := newPrototype()
	defer p.close()
	if *burst > 0 {
		p.burst(*burst)
		p.render(false)
		return
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		p.render(true)
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		switch strings.TrimSpace(strings.ToLower(line)) {
		case "s":
			p.surf()
		case "b":
			p.burst(50)
		case "q":
			return
		}
	}
}

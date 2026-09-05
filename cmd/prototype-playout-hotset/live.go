package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
)

// runLive answers the resource question the in-memory prototype cannot: how large is the
// transient process/FD/RSS/HLS footprint when a viewer surfs many copy-compatible channels inside
// one grace window? Sources are private synthetic lavfi streams, but both the session fan-out and
// HLS remux are production modules running real ffmpeg processes.
func runLive(channelCount int, ffmpeg string) error {
	if _, err := exec.LookPath(ffmpeg); err != nil {
		return err
	}
	workRoot, err := os.MkdirTemp("", "loomarr-hotset-live-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(workRoot) }()
	hlsBase := filepath.Join(workRoot, "hls")
	if err := os.Mkdir(hlsBase, 0o755); err != nil {
		return err
	}
	fixture := filepath.Join(workRoot, "copy-source.ts")
	if err := generateMPEGTSFixture(ffmpeg, fixture); err != nil {
		return err
	}

	var manager *playout.Manager
	spawn := func(ctx context.Context, channelID string, plan playout.EncodePlan) (*playout.Process, error) {
		proc, startErr := playout.Start(ctx, ffmpeg, copyMPEGTSArgs(fixture), nil, nil)
		if startErr == nil {
			// The fixture source and HLS packager both stream-copy. Reporting copy reproduces production
			// accounting: it releases the conservative admission slot while leaving both real processes.
			manager.ReportProgram(channelID, plan, playout.EncoderSoftware, false, playout.Progress{})
		}
		return proc, startErr
	}
	manager = playout.NewManager(spawn, func() int { return 1 }, playout.DefaultGrace, nil)
	hls, err := playout.NewHLSManager(manager, ffmpeg, hlsBase, playout.DefaultGrace, nil)
	if err != nil {
		manager.Stop()
		return err
	}

	sampler := newLiveSampler(os.Getpid(), hlsBase)
	sampler.start()
	started := time.Now()
	results := make(chan liveTune, channelCount)
	for index := 1; index <= channelCount; index++ {
		channelID := fmt.Sprintf("channel-%03d", index)
		go func() { results <- tuneLiveHLS(hls, channelID) }()
		// A tiny stagger preserves the user's sequential surf shape while keeping all starts inside
		// the 30-second grace window. Capacity retries below absorb any overlap in conservative cost.
		time.Sleep(10 * time.Millisecond)
	}

	tunes := make([]liveTune, 0, channelCount)
	for range channelCount {
		result := <-results
		if result.err != nil {
			hls.Stop()
			manager.Stop()
			sampler.stop()
			return fmt.Errorf("%s: %w", result.channel, result.err)
		}
		tunes = append(tunes, result)
	}
	close(results)
	sort.Slice(tunes, func(i, j int) bool { return tunes[i].channel < tunes[j].channel })

	// Keep the final channel viewer-active and turn every previous remux into a grace-idle sink.
	for index := range tunes[:len(tunes)-1] {
		tunes[index].detach()
	}
	currentDetach := tunes[len(tunes)-1].detach

	// Retune the immediately previous Channel: that is one of the two deliberately retained LRU
	// neighbors after the aggregate idle bound is enforced. The before-state retained every Channel,
	// so using tunes[0] accidentally measured an unlimited-hot-set behavior the fixed policy rejects.
	warmChannel := tunes[0].channel
	if len(tunes) > 1 {
		warmChannel = tunes[len(tunes)-2].channel
	}
	warm := make([]time.Duration, 0, 10)
	for range 10 {
		retuneStarted := time.Now()
		_, detach, retuneErr := hls.Playlist(warmChannel, playout.PlanBaseline)
		if retuneErr != nil {
			currentDetach()
			hls.Stop()
			manager.Stop()
			sampler.stop()
			return fmt.Errorf("warm retune: %w", retuneErr)
		}
		warm = append(warm, time.Since(retuneStarted))
		detach()
	}

	// Let process accounting observe a stable all-remuxes-running snapshot.
	time.Sleep(750 * time.Millisecond)
	steady := sampler.sample(true)
	peak := sampler.stop()
	stats := manager.Stats(time.Now())
	active, idle := splitSessions(stats)
	cold := make([]time.Duration, 0, len(tunes))
	for _, tune := range tunes {
		cold = append(cold, tune.latency)
	}

	fmt.Printf("%sLive ffmpeg warm-session battle test%s\n", bold, reset)
	fmt.Printf("%sQuestion:%s does copy-cost accounting bound total live process/resource use?\n\n", dim, reset)
	fmt.Printf("%sHost:%s %s / %s\n", bold, reset, runtimeOS(), ffmpegVersion(ffmpeg))
	fmt.Printf("%sCapacity:%s %s\n", bold, reset, hostCapacity())
	fmt.Printf("%sChannels visited:%s %d in %s\n", bold, reset, channelCount, time.Since(started).Round(time.Millisecond))
	fmt.Printf("%sLive sessions:%s %d (%d viewer-active, %d grace-idle)\n", bold, reset, len(stats), active, idle)
	fmt.Printf("%sTranscode budget:%s 1; every session reported video-copy cost 0\n", bold, reset)
	fmt.Printf("%sCold HLS manifest:%s p50 %s / p95 %s / max %s\n", bold, reset,
		percentile(cold, 0.50), percentile(cold, 0.95), percentile(cold, 1))
	fmt.Printf("%sWarm HLS retune:%s p50 %s / p95 %s / max %s\n", bold, reset,
		percentile(warm, 0.50), percentile(warm, 0.95), percentile(warm, 1))
	fmt.Printf("%sSteady ffmpeg processes after superseded viewers release:%s %d\n", bold, reset, steady.processes)
	fmt.Printf("%sSteady aggregate RSS / CPU / file descriptors / HLS scratch:%s %s / %.1f%% / %d / %s\n",
		bold, reset, bytesIEC(steady.rssKiB<<10), steady.cpuPercent, steady.fileDescriptors, bytesIEC(steady.hlsBytes))
	fmt.Printf("%sPeak ffmpeg processes during %d deliberately concurrent starts:%s %d\n",
		bold, channelCount, reset, peak.processes)
	fmt.Printf("%sPeak aggregate RSS:%s %s\n", bold, reset, bytesIEC(peak.rssKiB<<10))
	fmt.Printf("%sPeak aggregate CPU:%s %.1f%%\n", bold, reset, peak.cpuPercent)
	fmt.Printf("%sPeak ffmpeg file descriptors:%s %d\n", bold, reset, peak.fileDescriptors)
	fmt.Printf("%sPeak HLS scratch:%s %s\n", bold, reset, bytesIEC(peak.hlsBytes))

	currentDetach()
	hls.Stop()
	manager.Stop()
	time.Sleep(500 * time.Millisecond)
	cleaned := sampler.sample(true)
	fmt.Printf("%sAfter teardown:%s %d ffmpeg processes, %s HLS scratch\n", bold, reset,
		cleaned.processes, bytesIEC(cleaned.hlsBytes))
	return nil
}

func generateMPEGTSFixture(ffmpeg, destination string) error {
	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-f", "lavfi", "-i", "testsrc2=size=128x72:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=1000:sample_rate=48000", "-t", "8",
		"-map", "0:v:0", "-map", "1:a:0",
		"-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-threads", "1",
		"-pix_fmt", "yuv420p", "-g", "10", "-keyint_min", "10", "-sc_threshold", "0",
		"-b:v", "160k", "-maxrate", "160k", "-bufsize", "320k",
		"-c:a", "aac", "-b:a", "32k", "-ac", "2", "-ar", "48000",
		"-f", "mpegts", "-mpegts_flags", "+resend_headers", destination,
	}
	if body, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("generate copy fixture: %w: %s", err, strings.TrimSpace(string(body)))
	}
	return nil
}

func copyMPEGTSArgs(source string) []string {
	return []string{
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-stream_loop", "-1", "-re", "-i", source,
		"-map", "0:v:0", "-map", "0:a:0",
		"-c", "copy",
		"-f", "mpegts", "-mpegts_flags", "+resend_headers", "pipe:1",
	}
}

type liveTune struct {
	channel string
	latency time.Duration
	detach  func()
	err     error
}

func tuneLiveHLS(hls *playout.HLSManager, channelID string) liveTune {
	started := time.Now()
	deadline := started.Add(5 * time.Second)
	for {
		_, detach, err := hls.Playlist(channelID, playout.PlanBaseline)
		if !errors.Is(err, playout.ErrAtCapacity) || time.Now().After(deadline) {
			return liveTune{channel: channelID, latency: time.Since(started), detach: detach, err: err}
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func splitSessions(stats []playout.SessionStat) (active, idle int) {
	for _, stat := range stats {
		if stat.Viewers > 0 {
			active++
		} else {
			idle++
		}
	}
	return active, idle
}

func percentile(values []time.Duration, fraction float64) time.Duration {
	ordered := append([]time.Duration(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	if len(ordered) == 0 {
		return 0
	}
	index := int(math.Ceil(float64(len(ordered))*fraction)) - 1
	index = max(0, min(index, len(ordered)-1))
	return ordered[index].Round(time.Microsecond)
}

type liveSample struct {
	processes       int
	rssKiB          int64
	cpuPercent      float64
	fileDescriptors int
	hlsBytes        int64
}

func (s *liveSample) include(other liveSample) {
	s.processes = max(s.processes, other.processes)
	s.rssKiB = max(s.rssKiB, other.rssKiB)
	s.cpuPercent = max(s.cpuPercent, other.cpuPercent)
	s.fileDescriptors = max(s.fileDescriptors, other.fileDescriptors)
	s.hlsBytes = max(s.hlsBytes, other.hlsBytes)
}

type liveSampler struct {
	rootPID int
	hlsRoot string

	mu       sync.Mutex
	peak     liveSample
	stopOnce sync.Once
	stopCh   chan struct{}
	done     chan struct{}
}

func newLiveSampler(rootPID int, hlsRoot string) *liveSampler {
	return &liveSampler{rootPID: rootPID, hlsRoot: hlsRoot, stopCh: make(chan struct{}), done: make(chan struct{})}
}

func (s *liveSampler) start() {
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		iteration := 0
		for {
			select {
			case <-ticker.C:
				iteration++
				s.record(s.sample(iteration%5 == 0))
			case <-s.stopCh:
				s.record(s.sample(true))
				return
			}
		}
	}()
}

func (s *liveSampler) stop() liveSample {
	s.stopOnce.Do(func() { close(s.stopCh) })
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.peak
}

func (s *liveSampler) record(sample liveSample) {
	s.mu.Lock()
	s.peak.include(sample)
	s.mu.Unlock()
}

func (s *liveSampler) sample(withFDs bool) liveSample {
	processes := descendantFFmpeg(s.rootPID)
	sample := liveSample{processes: len(processes), hlsBytes: directoryBytes(s.hlsRoot)}
	for _, process := range processes {
		sample.rssKiB += process.rssKiB
		sample.cpuPercent += process.cpuPercent
	}
	if withFDs {
		sample.fileDescriptors = openFileDescriptors(processes)
	}
	return sample
}

type processSample struct {
	pid        int
	ppid       int
	rssKiB     int64
	cpuPercent float64
}

func descendantFFmpeg(rootPID int) []processSample {
	body, err := exec.Command("ps", "-axo", "pid=,ppid=,rss=,%cpu=,comm=").Output()
	if err != nil {
		return nil
	}
	all := make([]processSample, 0)
	commands := make([]string, 0)
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		ppid, ppidErr := strconv.Atoi(fields[1])
		rss, rssErr := strconv.ParseInt(fields[2], 10, 64)
		cpu, cpuErr := strconv.ParseFloat(fields[3], 64)
		if pidErr != nil || ppidErr != nil || rssErr != nil || cpuErr != nil {
			continue
		}
		all = append(all, processSample{pid: pid, ppid: ppid, rssKiB: rss, cpuPercent: cpu})
		commands = append(commands, strings.Join(fields[4:], " "))
	}
	descendants := map[int]bool{rootPID: true}
	for changed := true; changed; {
		changed = false
		for _, process := range all {
			if !descendants[process.pid] && descendants[process.ppid] {
				descendants[process.pid] = true
				changed = true
			}
		}
	}
	out := make([]processSample, 0)
	for index, process := range all {
		if descendants[process.pid] && strings.Contains(filepath.Base(commands[index]), "ffmpeg") {
			out = append(out, process)
		}
	}
	return out
}

func openFileDescriptors(processes []processSample) int {
	if len(processes) == 0 {
		return 0
	}
	pids := make([]string, 0, len(processes))
	for _, process := range processes {
		pids = append(pids, strconv.Itoa(process.pid))
	}
	body, err := exec.Command("lsof", "-a", "-p", strings.Join(pids, ","), "-Fn").Output()
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "n") {
			count++
		}
	}
	return count
}

func directoryBytes(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, infoErr := entry.Info(); infoErr == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func bytesIEC(value int64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	divisor, exponent := int64(unit), 0
	for quotient := value / unit; quotient >= unit; quotient /= unit {
		divisor *= unit
		exponent++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(divisor), "KMGTPE"[exponent])
}

func runtimeOS() string {
	body, err := exec.Command("uname", "-sm").Output()
	if err != nil {
		return "unknown host"
	}
	return strings.TrimSpace(string(body))
}

func hostCapacity() string {
	body, err := exec.Command("sysctl", "-n", "hw.model", "hw.ncpu", "hw.memsize").Output()
	if err != nil {
		return "not reported"
	}
	values := strings.Fields(string(body))
	if len(values) != 3 {
		return strings.TrimSpace(string(body))
	}
	memory, parseErr := strconv.ParseInt(values[2], 10, 64)
	if parseErr != nil {
		return strings.TrimSpace(string(body))
	}
	return fmt.Sprintf("%s, %s logical CPUs, %s memory", values[0], values[1], bytesIEC(memory))
}

func ffmpegVersion(ffmpeg string) string {
	body, err := exec.Command(ffmpeg, "-version").Output()
	if err != nil {
		return ffmpeg
	}
	line, _, _ := strings.Cut(string(body), "\n")
	return strings.TrimSpace(line)
}

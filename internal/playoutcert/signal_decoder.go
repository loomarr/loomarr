package playoutcert

import (
	"bufio"
	"context"
	"errors"
	"io"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// SignalDecoder extracts time-addressed video and audio measurements from one
// admitted media stream. Callbacks are invoked only before DecodeSignals returns.
type SignalDecoder interface {
	DecodeSignals(context.Context, io.ReadCloser, func(DecodedVideoSignal), func(DecodedAudioSignal)) error
}

type DecodedVideoSignal struct {
	PTSUS int64
	Luma  float64
}

type DecodedAudioSignal struct {
	PTSUS            int64
	Samples          int64
	ZeroCrossingRate float64
	// RMSDB is meaningful unless Silence is true. FFmpeg represents a silent
	// window as -inf, which is deliberately modeled as state rather than a
	// non-finite numeric measurement.
	RMSDB   float64
	Silence bool
}

// FFmpegSignalDecoder is a fixed-crop FFmpeg signal decoder. It is
// intentionally not wired into certification until its consumer exists.
type FFmpegSignalDecoder struct {
	Path    string
	command func(context.Context, string, ...string) *exec.Cmd
}

func (d FFmpegSignalDecoder) DecodeSignals(ctx context.Context, input io.ReadCloser, onVideo func(DecodedVideoSignal), onAudio func(DecodedAudioSignal)) error {
	var inputClose sync.Once
	closeInput := func() { inputClose.Do(func() { _ = input.Close() }) }
	path := strings.TrimSpace(d.Path)
	if path == "" {
		path = "ffmpeg"
	}
	command := d.command
	if command == nil {
		command = exec.CommandContext
	}
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := command(childCtx, path,
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-probesize", "256k", "-analyzeduration", "500000", "-copyts",
		"-i", "pipe:0", "-map", "0:v:0", "-map", "0:a:0",
		"-vf", "crop=32:32:16:16,settb=AVTB,signalstats,metadata=mode=print:key=lavfi.signalstats.YAVG:file='pipe\\:1'",
		"-af", "aresample=48000,aformat=channel_layouts=mono,asettb=AVTB,astats=metadata=1:reset=1,ametadata=mode=print:file='pipe\\:2'",
		"-fps_mode", "passthrough", "-f", "null", "-")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		closeInput()
		return errors.New("ffmpeg signal decoder stdin unavailable")
	}
	video, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		closeInput()
		return errors.New("ffmpeg signal decoder video pipe unavailable")
	}
	audio, err := cmd.StderrPipe()
	if err != nil {
		_ = video.Close()
		_ = stdin.Close()
		closeInput()
		return errors.New("ffmpeg signal decoder audio pipe unavailable")
	}
	if err := cmd.Start(); err != nil {
		_ = audio.Close()
		_ = video.Close()
		_ = stdin.Close()
		closeInput()
		return errors.New("ffmpeg signal decoder did not start")
	}

	copyDone := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(stdin, input)
		if closeErr := stdin.Close(); copyErr == nil {
			copyErr = closeErr
		}
		copyDone <- copyErr
	}()
	type parserResult struct{ err error }
	parseDone := make(chan parserResult, 2)
	go func() { parseDone <- parserResult{parseVideoSignals(video, onVideo)} }()
	go func() { parseDone <- parserResult{parseAudioSignals(audio, onAudio)} }()
	abort := func() {
		cancel()
		closeInput()
		_ = stdin.Close()
		_ = video.Close()
		_ = audio.Close()
	}

	var decodeErr error
	parsersRemaining, copyRemaining := 2, true
	for parsersRemaining > 0 || copyRemaining {
		select {
		case result := <-parseDone:
			parsersRemaining--
			if result.err != nil && decodeErr == nil {
				decodeErr = result.err
				// A parser failure must not wait on FFmpeg while the other pipe
				// remains writable.
				abort()
			}
			if parsersRemaining == 0 && decodeErr == nil && copyRemaining {
				// FFmpeg has closed both metadata pipes. Its child may already have
				// exited while an admitted streaming input remains blocked in Read;
				// release that reader before joining the copy goroutine.
				closeInput()
			}
		case copyErr := <-copyDone:
			copyRemaining = false
			// A deliberately closed blocked reader reports io.ErrClosedPipe. Any
			// other source error remains a failed decode even if FFmpeg has already
			// emitted complete metadata.
			if copyErr != nil && decodeErr == nil {
				decodeErr = copyErr
				// Input errors need the same prompt cancellation as parser errors:
				// otherwise FFmpeg can wait indefinitely for more input.
				abort()
			}
		}
	}
	// Both pipe readers have reached EOF before Wait, as required by os/exec.
	waitErr := cmd.Wait()
	closeInput()
	if decodeErr != nil || waitErr != nil {
		return errors.New("ffmpeg signal decoder failed")
	}
	return nil
}

var signalHeader = regexp.MustCompile(`^frame:([0-9]+)\s+pts:([-+]?[0-9]+)\s+pts_time:([^\s]+)\s*$`)

type signalRecord struct {
	frame, pts int64
	fields     map[string]string
}

func scanSignalRecords(reader io.Reader, consume func(signalRecord) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), 16<<10)
	var current *signalRecord
	var previousFrame, previousPTS int64
	seen := false
	flush := func() error {
		if current == nil {
			return nil
		}
		if (seen && current.frame <= previousFrame) || (seen && current.pts <= previousPTS) {
			return errors.New("signal metadata order invalid")
		}
		if err := consume(*current); err != nil {
			return err
		}
		previousFrame, previousPTS, seen = current.frame, current.pts, true
		current = nil
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "frame:") {
			if err := flush(); err != nil {
				return err
			}
			matches := signalHeader.FindStringSubmatch(line)
			if matches == nil {
				return errors.New("signal metadata header invalid")
			}
			frame, err1 := strconv.ParseInt(matches[1], 10, 64)
			pts, err2 := strconv.ParseInt(matches[2], 10, 64)
			ptsTime, err3 := strconv.ParseFloat(matches[3], 64)
			if err1 != nil || err2 != nil || err3 != nil || !finite(ptsTime) {
				return errors.New("signal metadata header invalid")
			}
			current = &signalRecord{frame: frame, pts: pts, fields: make(map[string]string)}
			continue
		}
		if current == nil || !strings.Contains(line, "=") {
			continue // bounded FFmpeg diagnostics on stderr are not operator output.
		}
		key, value, _ := strings.Cut(line, "=")
		if key == "" || len(current.fields) >= 128 {
			return errors.New("signal metadata record invalid")
		}
		if _, duplicate := current.fields[key]; duplicate {
			return errors.New("signal metadata record invalid")
		}
		current.fields[key] = value
	}
	if err := scanner.Err(); err != nil {
		return errors.New("signal metadata too large")
	}
	return flush()
}

func parseVideoSignals(reader io.Reader, callback func(DecodedVideoSignal)) error {
	count := 0
	err := scanSignalRecords(reader, func(record signalRecord) error {
		luma, ok := finiteFloat(record.fields["lavfi.signalstats.YAVG"])
		if !ok || luma < 0 || luma > 255 {
			return errors.New("video signal metadata invalid")
		}
		count++
		callback(DecodedVideoSignal{PTSUS: record.pts, Luma: luma})
		return nil
	})
	if err != nil || count == 0 {
		return errors.New("video signal metadata invalid")
	}
	return nil
}

func parseAudioSignals(reader io.Reader, callback func(DecodedAudioSignal)) error {
	count := 0
	err := scanSignalRecords(reader, func(record signalRecord) error {
		samples, ok := positiveInteger(record.fields["lavfi.astats.Overall.Number_of_samples"], 1048576)
		zeroRate, rateOK := finiteFloat(record.fields["lavfi.astats.1.Zero_crossings_rate"])
		rawRMS, rmsPresent := record.fields["lavfi.astats.1.RMS_level"]
		if !ok || !rateOK || zeroRate < 0 || zeroRate > 1 || !rmsPresent {
			return errors.New("audio signal metadata invalid")
		}
		signal := DecodedAudioSignal{PTSUS: record.pts, Samples: samples, ZeroCrossingRate: zeroRate}
		if strings.TrimSpace(rawRMS) == "-inf" {
			signal.Silence = true
		} else if rms, valid := finiteFloat(rawRMS); !valid || rms > 0 {
			return errors.New("audio signal metadata invalid")
		} else {
			signal.RMSDB = rms
		}
		count++
		callback(signal)
		return nil
	})
	if err != nil || count == 0 {
		return errors.New("audio signal metadata invalid")
	}
	return nil
}

func finiteFloat(raw string) (float64, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	return value, err == nil && finite(value)
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func positiveInteger(raw string, maximum int64) (int64, bool) {
	value, ok := finiteFloat(raw)
	if !ok || value != math.Trunc(value) || value <= 0 || value > float64(maximum) {
		return 0, false
	}
	return int64(value), true
}

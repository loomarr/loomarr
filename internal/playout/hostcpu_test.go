package playout

import (
	"testing"
	"testing/fstest"
)

func TestReadHostCPU(t *testing.T) {
	file := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	cases := []struct {
		name       string
		fs         fstest.MapFS
		wantCores  float64
		wantSource string
	}{
		{"cgroup v2 quota (docker --cpus 4)", fstest.MapFS{
			"proc/self/cgroup":      file("0::/\n"),
			"sys/fs/cgroup/cpu.max": file("400000 100000\n"),
		}, 4, "cgroup-v2"},
		{"cgroup v2 nested: the tightest ancestor wins", fstest.MapFS{
			"proc/self/cgroup":                                   file("0::/system.slice/loomarr.service\n"),
			"sys/fs/cgroup/system.slice/cpu.max":                 file("150000 100000\n"),
			"sys/fs/cgroup/system.slice/loomarr.service/cpu.max": file("max 100000\n"),
		}, 1.5, "cgroup-v2"},
		{"cgroup v2 unlimited", fstest.MapFS{
			"proc/self/cgroup":      file("0::/\n"),
			"sys/fs/cgroup/cpu.max": file("max 100000\n"),
		}, 8, "cpu-count"},
		{"cgroup v1 cfs quota", fstest.MapFS{
			"proc/self/cgroup":                            file("12:memory:/docker/abc\n4:cpu,cpuacct:/docker/abc\n"),
			"sys/fs/cgroup/cpu,cpuacct/cpu.cfs_quota_us":  file("250000\n"),
			"sys/fs/cgroup/cpu,cpuacct/cpu.cfs_period_us": file("100000\n"),
		}, 2.5, "cgroup-v1"},
		{"cgroup v1 unlimited (-1)", fstest.MapFS{
			"proc/self/cgroup":                            file("4:cpu,cpuacct:/\n"),
			"sys/fs/cgroup/cpu,cpuacct/cpu.cfs_quota_us":  file("-1\n"),
			"sys/fs/cgroup/cpu,cpuacct/cpu.cfs_period_us": file("100000\n"),
		}, 8, "cpu-count"},
		{"no cgroup (macOS, bare metal)", fstest.MapFS{}, 8, "cpu-count"},
		{"quota above the CPU count", fstest.MapFS{
			"proc/self/cgroup":      file("0::/\n"),
			"sys/fs/cgroup/cpu.max": file("1600000 100000\n"),
		}, 8, "cpu-count"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ReadHostCPU(tc.fs, 8)
			if got.Cores != tc.wantCores || got.Source != tc.wantSource {
				t.Fatalf("ReadHostCPU = %+v, want %v cores from %s", got, tc.wantCores, tc.wantSource)
			}
		})
	}
}

func TestPlayoutCPUAllowance(t *testing.T) {
	host := HostCPU{Cores: 4, Source: "cgroup-v2"}
	cases := []struct {
		name                   string
		hardware               bool
		gpuMilli, reserveMilli int
		want                   float64
	}{
		{"GPU host defaults to one core", true, 1000, 1000, 1},
		{"GPU host setting", true, 1500, 1000, 1.5},
		{"GPU host setting above the host", true, 9000, 1000, 4},
		{"software host keeps a core for the app", false, 1000, 1000, 3},
		{"software host reserve setting", false, 1000, 500, 3.5},
		{"software host smaller than its reserve still gets half a core", false, 1000, 8000, 0.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PlayoutCPUAllowance(host, tc.hardware, tc.gpuMilli, tc.reserveMilli); got != tc.want {
				t.Fatalf("allowance = %v, want %v", got, tc.want)
			}
		})
	}
}

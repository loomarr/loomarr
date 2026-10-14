package playout

import (
	"io/fs"
	"path"
	"strconv"
	"strings"
)

// HostCPU is the CPU this process may use: the cgroup quota in a container, else the CPU count.
type HostCPU struct {
	Cores  float64 `json:"cores"`
	Source string  `json:"source"` // cgroup-v2 | cgroup-v1 | cpu-count
}

// ReadHostCPU reads the applicable CPU limit from root (the filesystem root, "/" in production).
// A quota tighter than numCPU wins; no quota, or a platform without cgroups, is the CPU count.
func ReadHostCPU(root fs.FS, numCPU int) HostCPU {
	out := HostCPU{Cores: float64(numCPU), Source: "cpu-count"}
	raw, err := fs.ReadFile(root, "proc/self/cgroup")
	if err != nil {
		return out
	}
	quota, source := 0.0, ""
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		switch {
		case parts[0] == "0" && parts[1] == "":
			if q, ok := cgroupV2Quota(root, parts[2]); ok {
				quota, source = q, "cgroup-v2"
			}
		case hasController(parts[1], "cpu"):
			if q, ok := cgroupV1Quota(root, parts[1], parts[2]); ok {
				quota, source = q, "cgroup-v1"
			}
		}
	}
	if quota > 0 && quota < out.Cores {
		return HostCPU{Cores: quota, Source: source}
	}
	return out
}

func hasController(list, name string) bool {
	for _, c := range strings.Split(list, ",") {
		if c == name {
			return true
		}
	}
	return false
}

// cgroupV2Quota walks from the process's cgroup to the root and returns the tightest cpu.max.
// Inside a container the namespace root is the container's own cgroup, so "/" is the usual case.
func cgroupV2Quota(root fs.FS, group string) (float64, bool) {
	best, found := 0.0, false
	for dir := path.Clean("/" + group); ; dir = path.Dir(dir) {
		if raw, err := fs.ReadFile(root, path.Join("sys/fs/cgroup", dir, "cpu.max")); err == nil {
			fields := strings.Fields(string(raw))
			if len(fields) == 2 && fields[0] != "max" {
				q, qerr := strconv.ParseFloat(fields[0], 64)
				p, perr := strconv.ParseFloat(fields[1], 64)
				if qerr == nil && perr == nil && q > 0 && p > 0 && (!found || q/p < best) {
					best, found = q/p, true
				}
			}
		}
		if dir == "/" {
			return best, found
		}
	}
}

// cgroupV1Quota reads cpu.cfs_quota_us / cpu.cfs_period_us from the controller's mount, at the
// process's cgroup and then at the mount root (a container sees its own cgroup there).
func cgroupV1Quota(root fs.FS, controllers, group string) (float64, bool) {
	for _, mount := range []string{controllers, "cpu,cpuacct", "cpu"} {
		for _, dir := range []string{group, "/"} {
			base := path.Join("sys/fs/cgroup", mount, dir)
			q, qerr := readInt(root, path.Join(base, "cpu.cfs_quota_us"))
			p, perr := readInt(root, path.Join(base, "cpu.cfs_period_us"))
			if qerr != nil || perr != nil {
				continue
			}
			if q <= 0 || p <= 0 {
				return 0, false // -1: no quota
			}
			return float64(q) / float64(p), true
		}
	}
	return 0, false
}

func readInt(root fs.FS, name string) (int64, error) {
	raw, err := fs.ReadFile(root, name)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
}

// minSoftwareAllowance keeps a software host that is smaller than its app reserve able to play one
// cheap stream rather than none.
const minSoftwareAllowance = 0.5

// PlayoutCPUAllowance is the cores playout may use (#1512 G5): on a GPU host
// playout.gpu_cpu_millicores (default one core, the maintainer's total), on a software host the
// host total less playout.app_reserve_millicores (default one core kept for the app). Neither
// exceeds the host.
func PlayoutCPUAllowance(host HostCPU, hardware bool, gpuMillicores, appReserveMillicores int) float64 {
	if hardware {
		return min(float64(gpuMillicores)/1000, host.Cores)
	}
	return min(max(host.Cores-float64(appReserveMillicores)/1000, minSoftwareAllowance), host.Cores)
}

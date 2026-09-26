package playout

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// clockTicks is USER_HZ, which Linux fixes at 100 for /proc on every mainstream architecture.
const clockTicks = 100

// processCPUTime is a running process's user+system CPU time from /proc/<pid>/stat.
func processCPUTime(pid int) (time.Duration, bool) {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, false
	}
	return parseProcStatCPU(string(raw))
}

// parseProcStatCPU reads utime and stime (fields 14 and 15). The command name (field 2) may hold
// spaces and parentheses, so fields are counted after its closing parenthesis.
func parseProcStatCPU(stat string) (time.Duration, bool) {
	end := strings.LastIndexByte(stat, ')')
	if end < 0 {
		return 0, false
	}
	fields := strings.Fields(stat[end+1:])
	// fields[0] is field 3 (state), so utime (14) is fields[11] and stime (15) fields[12].
	if len(fields) < 13 {
		return 0, false
	}
	utime, err1 := strconv.ParseInt(fields[11], 10, 64)
	stime, err2 := strconv.ParseInt(fields[12], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return time.Duration(utime+stime) * time.Second / clockTicks, true
}

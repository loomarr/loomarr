# Shield certification analyser (#1037). Reads `adb logcat -v epoch` output holding the TV app's
# `LoomarrCert key=value` marks (tag ReactNativeJS) plus the framework's codec lines, and writes
# JSON (to the file named by `json`) and a plain summary (stdout).
#
# POSIX awk only (macOS ships BWK awk): no gawk extensions, no array length, no asort.
#
# Variables (awk -v):
#   mode        surf | soak
#   json        path of the JSON report to write
#   codec_re    ERE matching one decoder instantiation line (see docs/engineering/shield-certification.md)
#   end_epoch   soak: wall-clock end of the run, seconds (the last attempt is watched until then)
#   grace_ms    soak: a codec init this long after an attempt's first frame is mid-play (default 2000)
#   reconnects  soak: adb reconnects survived, reported as is
#   target_h    soak: viewer-hours the gate needs (default 24)
#
# Clocks: mark `t=` values are one monotonic clock per app process (pid) and give every latency.
# Codec lines carry only the logcat wall clock, so they are placed against marks by each pid's
# offset between the two clocks.

function isnum(value) { return value ~ /^-?[0-9]+(\.[0-9]+)?$/ }

function sort_numeric(values, size,    at, before, value) {
	for (at = 2; at <= size; at++) {
		value = values[at]
		before = at - 1
		while (before > 0 && values[before] > value) {
			values[before + 1] = values[before]
			before--
		}
		values[before + 1] = value
	}
}

# Nearest-rank percentile over list `name` (`count[name]` values). A surf that never reached its
# first frame is stored as UNFINISHED so it can only raise a percentile, never hide.
function percentile(name, pct,    n, at, rank, sorted) {
	n = count[name] + 0
	if (n == 0) return ""
	for (at = 1; at <= n; at++) sorted[at] = list[name, at]
	sort_numeric(sorted, n)
	rank = int((pct * n + 99) / 100)
	if (rank < 1) rank = 1
	if (rank > n) rank = n
	return sorted[rank]
}

function push(name, value) {
	count[name]++
	list[name, count[name]] = value
}

function fmt(value) {
	if (value == "") return "null"
	if (value >= UNFINISHED) return "null"
	return sprintf("%.0f", value)
}

function human(value) {
	if (value == "") return "n/a"
	if (value >= UNFINISHED) return "unfinished"
	return sprintf("%.0f ms", value)
}

function stats_json(name,    s) {
	s = sprintf("{\"n\":%d,\"p50\":%s,\"p95\":%s,\"max\":%s}", count[name] + 0,
		fmt(percentile(name, 50)), fmt(percentile(name, 95)), fmt(percentile(name, 100)))
	return s
}

# PASS / FAIL / NO-DATA against a p95 ceiling in ms.
function verdict(name, ceiling,    p95) {
	p95 = percentile(name, 95)
	if (p95 == "") return "NO-DATA"
	return (p95 <= ceiling) ? "PASS" : "FAIL"
}

BEGIN {
	UNFINISHED = 1e12
	if (grace_ms == "") grace_ms = 2000
	if (target_h == "") target_h = 24
	attempts = 0
	codecs = 0
	split("", stall_open)
	surf_reason["step"] = 1
	surf_reason["number"] = 1
	surf_reason["channel"] = 1
	surf_reason["previous"] = 1
}

{
	mark = 0
	for (f = 1; f <= NF; f++) {
		if ($f == "LoomarrCert") {
			mark = f
			break
		}
	}
	if (!mark) {
		if (codec_re != "" && $0 ~ codec_re && isnum($1)) {
			codecs++
			codec_at[codecs] = $1 * 1000
			codec_pid[codecs] = $2
			codec_track[codecs] = ($0 ~ /track type (video|audio)$/) ? $NF : "other"
		}
		next
	}
	pid = $2
	logged_ms = $1 * 1000
	delete kv
	for (f = mark + 1; f <= NF; f++) {
		eq = index($f, "=")
		if (eq > 1) kv[substr($f, 1, eq - 1)] = substr($f, eq + 1)
	}
	if (kv["v"] != "1" || !isnum(kv["t"])) next
	t = kv["t"] + 0
	event = kv["ev"]
	if (!(pid in seen_pid)) {
		processes++
		process_order[processes] = pid
		first_wall[pid] = logged_ms
	}
	# The newest mark fixes this process's wall-minus-monotonic offset.
	offset[pid] = logged_ms - t
	seen_pid[pid] = 1
	last_t[pid] = t
	id = pid ":" kv["att"]

	if (event == "tune") {
		attempts++
		order[attempts] = id
		a_pid[id] = pid
		a_tune[id] = t
		a_key[id] = isnum(kv["key"]) ? kv["key"] + 0 : ""
		a_path[id] = kv["path"]
		a_why[id] = kv["why"]
		a_ch[id] = kv["ch"]
		a_num[id] = kv["num"]
	} else if (event == "first-frame") {
		if (!(id in a_ff)) a_ff[id] = t
	} else if (event == "held") {
		if (kv["what"] == "osd" && !(id in a_osd)) a_osd[id] = t
		if (kv["what"] == "still" && !(id in a_still)) a_still[id] = t
	} else if (event == "stall-start") {
		stalls++
		stall_open[pid] = 1
	} else if (event == "stall-end") {
		stall_open[pid] = 0
		if (isnum(kv["dur"])) {
			push("stall", kv["dur"] + 0)
			stall_ms += kv["dur"]
		}
	} else if (event == "error") {
		errors++
		cause = kv["cause"]
		if (!(id in a_err)) a_err[id] = cause
		if (!(cause in error_by)) {
			error_causes++
			error_name[error_causes] = cause
		}
		error_by[cause]++
	} else if (event == "format" && kv["mime"] != "none") {
		# A replace reports no track ("none") before the new one: that is the tune, not a change.
		# The first picture's height sets which cold ceiling the surf is judged against.
		if (kv["mime"] ~ /^video\// && !(id in a_height) && isnum(kv["h"])) a_height[id] = kv["h"] + 0
		signature = kv["mime"] "/" kv["w"] "x" kv["h"]
		if ((id in a_format) && a_format[id] != signature) format_changes++
		a_format[id] = signature
	}
}

END {
	# A process that was replaced (the app restarted) stopped playing by the time its successor
	# wrote its first mark; the newest process plays until the end of the run.
	for (p = 1; p <= processes; p++) {
		pid = process_order[p]
		if (p < processes) stop_wall[pid] = first_wall[process_order[p + 1]]
		else if (end_epoch != "") stop_wall[pid] = end_epoch * 1000
		else stop_wall[pid] = last_t[pid] + offset[pid]
	}
	# Each attempt ends where the next one in its process starts, or when its process stopped.
	for (i = 1; i <= attempts; i++) {
		id = order[i]
		a_end[id] = ""
		for (j = i + 1; j <= attempts; j++) {
			if (a_pid[order[j]] == a_pid[id]) {
				a_end[id] = a_tune[order[j]]
				break
			}
		}
		if (a_end[id] == "") a_end[id] = stop_wall[a_pid[id]] - offset[a_pid[id]]
	}

	if (mode == "surf") {
		boot = 0
		unfinished = 0
		refused = 0
		for (i = 1; i <= attempts; i++) {
			id = order[i]
			# The player's own retries belong to the surf they follow.
			if (a_why[id] == "retry") continue
			if (!(a_why[id] in surf_reason)) {
				boot++
				continue
			}
			# A number entry commits after the app's deliberate 1.2 s wait (or on OK): time it from
			# that commit. Every other surf is timed from its key.
			start = (a_key[id] != "" && a_why[id] != "number") ? a_key[id] : a_tune[id]
			surfs++
			# A surf is served by its own first frame or a retry's; one that never framed after an
			# error mark was refused (first cause wins); one with neither simply ran out of time.
			framed = (id in a_ff) ? a_ff[id] : ""
			served = id
			cause = (id in a_err) ? a_err[id] : ""
			for (j = i + 1; framed == "" && j <= attempts; j++) {
				retry = order[j]
				if (a_pid[retry] != a_pid[id] || a_why[retry] != "retry") break
				if (retry in a_ff) {
					framed = a_ff[retry]
					served = retry
				}
				if (cause == "" && (retry in a_err)) cause = a_err[retry]
			}
			# Warm has one ceiling for every format. Cold allows a 4K premium (2160 lines or more)
			# its own; a cold surf that never reported a picture is held to the baseline.
			if (a_path[id] == "warm") path = "warm"
			else if (a_height[served] >= 2160) path = "cold4k"
			else path = "cold"
			if (framed != "") {
				push(path, framed - start)
			} else if (cause != "") {
				# Refused surfs have no latency; the refusal gate fails on them instead.
				refused++
				if (!(cause in refused_by)) {
					refused_causes++
					refused_name[refused_causes] = cause
				}
				refused_by[cause]++
			} else {
				push(path, UNFINISHED)
				unfinished++
			}
			if (id in a_osd) push("held", a_osd[id] - start)
			if (id in a_still) push("still", a_still[id] - start)
			if (a_key[id] == "") unkeyed++
			else if (a_why[id] != "number") push("dispatch", a_tune[id] - a_key[id])
		}
		g_warm = verdict("warm", 600)
		g_cold = verdict("cold", 1500)
		g_cold4k = verdict("cold4k", 2500)
		g_held = verdict("held", 100)
		g_refused = (surfs + 0 == 0) ? "NO-DATA" : (refused ? "FAIL" : "PASS")
		refused_json = ""
		refused_text = ""
		for (e = 1; e <= refused_causes; e++) {
			refused_json = refused_json sprintf("%s\"%s\":%d", (e > 1 ? "," : ""), refused_name[e], refused_by[refused_name[e]])
			refused_text = refused_text sprintf("%s%s x%d", (e > 1 ? ", " : ""), refused_name[e], refused_by[refused_name[e]])
		}
		printf "{\"mode\":\"surf\",\"surfs\":%d,\"unfinished\":%d,\"refused\":%d,\"refusedBy\":{%s},\"unkeyed\":%d,\"otherTunes\":%d,", surfs, unfinished, refused, refused_json, unkeyed + 0, boot > json
		printf "\"warm\":%s,\"cold\":%s,\"cold4k\":%s,\"held\":%s,\"still\":%s,\"keyToTune\":%s,", stats_json("warm"), stats_json("cold"), stats_json("cold4k"), stats_json("held"), stats_json("still"), stats_json("dispatch") > json
		printf "\"stalls\":%d,\"errors\":%d,", stalls + 0, errors + 0 > json
		printf "\"gates\":{\"warmP95Max600\":\"%s\",\"coldP95Max1500\":\"%s\",\"cold4kP95Max2500\":\"%s\",\"heldP95Max100\":\"%s\",\"refusedMax0\":\"%s\"}}\n", g_warm, g_cold, g_cold4k, g_held, g_refused > json

		printf "surfs            %d (%d refused, %d unfinished before the next key, %d without a key time; %d other tunes ignored)\n", surfs, refused, unfinished, unkeyed + 0, boot
		printf "refused          %d%s   [G3 none refused: %s]\n", refused, (refused_text == "" ? "" : " (" refused_text ")"), g_refused
		printf "warm key->frame  n=%d p50 %s  p95 %s  max %s   [G3 p95 <= 600 ms: %s]\n", count["warm"] + 0, human(percentile("warm", 50)), human(percentile("warm", 95)), human(percentile("warm", 100)), g_warm
		printf "cold key->frame  n=%d p50 %s  p95 %s  max %s   [G3 p95 <= 1500 ms: %s]\n", count["cold"] + 0, human(percentile("cold", 50)), human(percentile("cold", 95)), human(percentile("cold", 100)), g_cold
		printf "cold 4K premium  n=%d p50 %s  p95 %s  max %s   [G3 p95 <= 2500 ms: %s]\n", count["cold4k"] + 0, human(percentile("cold4k", 50)), human(percentile("cold4k", 95)), human(percentile("cold4k", 100)), g_cold4k
		printf "held OSD         n=%d p50 %s  p95 %s  max %s   [G3 p95 <= 100 ms: %s]\n", count["held"] + 0, human(percentile("held", 50)), human(percentile("held", 95)), human(percentile("held", 100)), g_held
		printf "held still       n=%d p50 %s  p95 %s  max %s\n", count["still"] + 0, human(percentile("still", 50)), human(percentile("still", 95)), human(percentile("still", 100))
		printf "key->tune (JS)   n=%d p50 %s  p95 %s\n", count["dispatch"] + 0, human(percentile("dispatch", 50)), human(percentile("dispatch", 95))
		printf "stalls %d  errors %d\n", stalls + 0, errors + 0
		exit
	}

	# Soak: time watched is first frame to the attempt's end; codec inits are placed by wall clock.
	for (i = 1; i <= attempts; i++) {
		id = order[i]
		if (!(id in a_ff)) continue
		watched = a_end[id] - a_ff[id]
		if (watched > 0) viewer_ms += watched
	}
	for (c = 1; c <= codecs; c++) {
		pid = codec_pid[c]
		if (!(pid in seen_pid)) continue
		app_codecs++
		when = codec_at[c] - offset[pid]
		placed = 0
		for (i = 1; i <= attempts; i++) {
			id = order[i]
			if (a_pid[id] != pid) continue
			settle = (id in a_ff) ? a_ff[id] + grace_ms : a_end[id]
			if (when >= a_tune[id] - 500 && when <= settle) {
				placed = 1
				break
			}
		}
		if (placed) tune_codecs++
		else {
			midplay_codecs++
			midplay_by[codec_track[c]]++
		}
	}
	for (pid in stall_open) if (stall_open[pid]) open_stalls++
	hours = viewer_ms / 3600000
	rate = (hours > 0) ? stalls / hours : ""

	if (attempts > 0 && app_codecs + 0 == 0) codec_state = "UNVERIFIED"
	else codec_state = "COUNTED"
	if (hours <= 0) g4 = "NO-DATA"
	else if (codec_state != "COUNTED") g4 = "UNVERIFIED"
	else if (rate >= 1 || midplay_codecs + 0 > 0) g4 = "FAIL"
	else if (hours < target_h) g4 = "INCOMPLETE"
	else g4 = "PASS"

	errors_json = ""
	for (e = 1; e <= error_causes; e++) {
		errors_json = errors_json sprintf("%s\"%s\":%d", (e > 1 ? "," : ""), error_name[e], error_by[error_name[e]])
	}
	printf "{\"mode\":\"soak\",\"viewerHours\":%.3f,\"targetHours\":%s,\"tunes\":%d,\"appProcesses\":%d,\"adbReconnects\":%d,", hours, target_h, attempts, processes, reconnects + 0 > json
	printf "\"stalls\":%d,\"openStalls\":%d,\"stallsPerViewerHour\":%s,\"stallMs\":%s,", stalls + 0, open_stalls + 0, (rate == "" ? "null" : sprintf("%.3f", rate)), stats_json("stall") > json
	printf "\"codecInits\":{\"state\":\"%s\",\"total\":%d,\"atTune\":%d,\"midPlay\":%d,\"midPlayVideo\":%d,\"midPlayAudio\":%d},\"formatChangesMidAttempt\":%d,", codec_state, app_codecs + 0, tune_codecs + 0, midplay_codecs + 0, midplay_by["video"] + 0, midplay_by["audio"] + 0, format_changes + 0 > json
	printf "\"errors\":{%s},\"gate\":\"%s\"}\n", errors_json, g4 > json

	printf "viewer time        %.2f h of %s h (%d tunes, %d app processes, %d adb reconnects)\n", hours, target_h, attempts, processes, reconnects + 0
	printf "stalls             %d (%s per viewer-hour; %d still open; stall p95 %s)\n", stalls + 0, (rate == "" ? "n/a" : sprintf("%.3f", rate)), open_stalls + 0, human(percentile("stall", 95))
	printf "codec inits        %d total: %d at tunes, %d mid-play (%d video, %d audio) [%s]\n", app_codecs + 0, tune_codecs + 0, midplay_codecs + 0, midplay_by["video"] + 0, midplay_by["audio"] + 0, codec_state
	printf "format changes     %d inside an attempt\n", format_changes + 0
	printf "errors             %d\n", errors + 0
	printf "G4 (< 1 stall/viewer-hour over %s h, 0 mid-play codec re-inits): %s\n", target_h, g4
}

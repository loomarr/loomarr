# Heavy-lock session: build the phase 0 Exo app, boot the ATV emulator, play the HEVC sets and the DVR delta playlist.
set -u; E=$(dirname "$0"); P=$E/../play; export ANDROID_HOME=$HOME/Android ANDROID_SDK_ROOT=$HOME/Android
G=$(ls -d ~/.gradle/wrapper/dists/gradle-8.11.1*/*/gradle-8.11.1/bin/gradle | head -1)
(cd $E && timeout 600 $G -q --offline assembleDebug 2>&1 | tail -3) ; APK=$(ls $E/app/build/outputs/apk/debug/*.apk 2>/dev/null | head -1); echo "apk=$APK"; [ -n "$APK" ] || exit 1
$ANDROID_HOME/emulator/emulator -avd loomarr-tv -no-window -no-audio -no-snapshot -gpu swiftshader_indirect >/dev/null 2>&1 & EM=$!
adb wait-for-device; timeout 180 bash -c 'until [ "$(adb shell getprop sys.boot_completed | tr -d "\r")" = 1 ]; do sleep 2; done'
adb install -r "$APK" >/dev/null && echo installed
adb shell getprop ro.build.version.release; adb shell dumpsys media.player 2>/dev/null | head -0
play() { adb logcat -c; adb shell am start -S -n dev.loomarr.spike/.MainActivity --es url "$1" >/dev/null; sleep $2; adb logcat -d -s SPK:I | grep -vE '^---' | cut -c1-220 > $E/log-$3.txt; adb shell am force-stop dev.loomarr.spike; echo "== $3"; grep -E 'vdec_init|format|error|Error|rebuffer|state|drop' $E/log-$3.txt | head -14; }
(cd $P && PORT=8765 node server.cjs > srv-exo-hevc.log & echo $! > $E/srv.pid); sleep 1
play http://10.0.2.2:8765/hdr/index.m3u8 40 hdr; play http://10.0.2.2:8765/sdr/index.m3u8 45 sdr; kill $(cat $E/srv.pid)
(cd $P && SKIP=1 PORT=8765 node server.cjs > srv-exo-dvr.log & echo $! > $E/srv.pid); sleep 1
play http://10.0.2.2:8765/dvr/live.m3u8 40 dvr; kill $(cat $E/srv.pid)
echo "exo dvr: $(grep -c . $P/srv-exo-dvr.log) playlist reqs, skip=$(grep -c '"skip":true' $P/srv-exo-dvr.log), bytes: $(grep -o '"bytes":[0-9]*' $P/srv-exo-dvr.log | cut -d: -f2 | sort -n | uniq -c | sort -rn | head -2 | tr '\n' ' ')"
adb emu kill >/dev/null 2>&1; sleep 3; kill $EM 2>/dev/null; true

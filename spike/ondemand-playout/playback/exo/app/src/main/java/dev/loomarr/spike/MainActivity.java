package dev.loomarr.spike;

// Phase-0 spike (#1512): play the spike HLS in ExoPlayer, the engine expo-video wraps on Android, with
// bufferForPlaybackMs (expo-video's minBufferForPlayback) at ~1 s. Everything lands in logcat tag SPK:
// first frame, rebuffers, and every MediaCodec init/release, which is what the break test counts.

import android.app.Activity;
import android.net.Uri;
import android.os.Bundle;
import android.os.SystemClock;
import android.util.Log;
import android.view.SurfaceView;
import androidx.media3.common.Format;
import androidx.media3.common.MediaItem;
import androidx.media3.common.PlaybackException;
import androidx.media3.common.Player;
import androidx.media3.exoplayer.DecoderReuseEvaluation;
import androidx.media3.exoplayer.DefaultLoadControl;
import androidx.media3.exoplayer.ExoPlayer;
import androidx.media3.exoplayer.analytics.AnalyticsListener;

public class MainActivity extends Activity {
  private ExoPlayer player;
  private long t0;
  private boolean ready;
  private int rebuffers;

  private void log(String s) {
    Log.i("SPK", "t_ms=" + (SystemClock.elapsedRealtime() - t0) + " " + s);
  }

  @Override
  protected void onCreate(Bundle b) {
    super.onCreate(b);
    t0 = SystemClock.elapsedRealtime();
    SurfaceView sv = new SurfaceView(this);
    setContentView(sv);
    String url = getIntent().getStringExtra("url");
    int buf = getIntent().getIntExtra("buf", 1000);
    DefaultLoadControl lc = new DefaultLoadControl.Builder()
        .setBufferDurationsMs(DefaultLoadControl.DEFAULT_MIN_BUFFER_MS, DefaultLoadControl.DEFAULT_MAX_BUFFER_MS, buf, 2 * buf)
        .build();
    player = new ExoPlayer.Builder(this).setLoadControl(lc).build();
    player.setVideoSurfaceView(sv);
    player.addAnalyticsListener(new AnalyticsListener() {
      @Override public void onPlaybackStateChanged(EventTime e, int state) {
        if (state == Player.STATE_READY) ready = true;
        if (state == Player.STATE_BUFFERING && ready) rebuffers++;
        log("state=" + state + " rebuffers=" + rebuffers + " pos_ms=" + e.currentPlaybackPositionMs);
      }
      @Override public void onRenderedFirstFrame(EventTime e, Object output, long renderTimeMs) { log("first_frame"); }
      @Override public void onVideoDecoderInitialized(EventTime e, String name, long initializedTimestampMs, long initDurationMs) { log("vdec_init " + name + " init_ms=" + initDurationMs); }
      @Override public void onVideoDecoderReleased(EventTime e, String name) { log("vdec_release " + name); }
      @Override public void onAudioDecoderInitialized(EventTime e, String name, long initializedTimestampMs, long initDurationMs) { log("adec_init " + name); }
      @Override public void onAudioDecoderReleased(EventTime e, String name) { log("adec_release " + name); }
      @Override public void onVideoInputFormatChanged(EventTime e, Format f, DecoderReuseEvaluation r) {
        log("vformat " + f.width + "x" + f.height + " " + f.codecs + " reuse=" + (r == null ? "none" : r.result + "/" + r.discardReasons) + " pos_ms=" + e.currentPlaybackPositionMs);
      }
      @Override public void onAudioInputFormatChanged(EventTime e, Format f, DecoderReuseEvaluation r) {
        log("aformat " + f.sampleRate + " " + f.codecs + " reuse=" + (r == null ? "none" : r.result + "/" + r.discardReasons));
      }
      @Override public void onDroppedVideoFrames(EventTime e, int dropped, long elapsedMs) { log("dropped=" + dropped); }
      @Override public void onPlayerError(EventTime e, PlaybackException err) { log("error " + err.getErrorCodeName() + " " + err.getMessage()); }
    });
    log("start url=" + url + " bufferForPlaybackMs=" + buf);
    player.setMediaItem(MediaItem.fromUri(Uri.parse(url)));
    player.setPlayWhenReady(true);
    player.prepare();
  }

  @Override
  protected void onDestroy() {
    log("end rebuffers=" + rebuffers);
    player.release();
    super.onDestroy();
  }
}

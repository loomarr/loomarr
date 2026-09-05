package media.loomarr.tv.discovery;

import android.content.Context;
import android.net.nsd.NsdManager;
import android.net.nsd.NsdServiceInfo;
import com.facebook.react.bridge.Arguments;
import com.facebook.react.bridge.ReactApplicationContext;
import com.facebook.react.bridge.ReactContextBaseJavaModule;
import com.facebook.react.bridge.ReactMethod;
import com.facebook.react.bridge.WritableMap;
import com.facebook.react.modules.core.DeviceEventManagerModule;
import java.net.InetAddress;
import java.nio.charset.StandardCharsets;
import java.util.Map;

public final class LoomarrLanDiscoveryModule extends ReactContextBaseJavaModule {
  private static final String SERVICE_TYPE = "_loomarr._tcp.";
  private final NsdManager manager;
  private NsdManager.DiscoveryListener listener;

  LoomarrLanDiscoveryModule(ReactApplicationContext context) {
    super(context);
    manager = (NsdManager) context.getSystemService(Context.NSD_SERVICE);
  }

  @Override
  public String getName() {
    return "LoomarrLanDiscovery";
  }

  @ReactMethod
  public void start() {
    stop();
    listener = new NsdManager.DiscoveryListener() {
      @Override public void onDiscoveryStarted(String type) {}
      @Override public void onDiscoveryStopped(String type) {}

      @Override
      public void onStartDiscoveryFailed(String type, int code) {
        emitError(code);
        stop();
      }

      @Override
      public void onStopDiscoveryFailed(String type, int code) {
        emitError(code);
        listener = null;
      }

      @Override
      public void onServiceFound(NsdServiceInfo service) {
        if (!SERVICE_TYPE.equals(service.getServiceType())) return;
        manager.resolveService(service, new NsdManager.ResolveListener() {
          @Override public void onResolveFailed(NsdServiceInfo ignored, int code) { emitError(code); }
          @Override public void onServiceResolved(NsdServiceInfo resolved) { emitFound(resolved); }
        });
      }

      @Override
      public void onServiceLost(NsdServiceInfo service) {
        WritableMap payload = Arguments.createMap();
        payload.putString("id", service.getServiceName());
        emit("loomarrDiscoveryLost", payload);
      }
    };
    manager.discoverServices(SERVICE_TYPE, NsdManager.PROTOCOL_DNS_SD, listener);
  }

  @ReactMethod
  public void stop() {
    NsdManager.DiscoveryListener active = listener;
    listener = null;
    if (active == null) return;
    try {
      manager.stopServiceDiscovery(active);
    } catch (IllegalArgumentException ignored) {
      // Android throws when discovery failed before registration completed; stopped is still true.
    }
  }

  @ReactMethod
  public void addListener(String eventName) {}

  @ReactMethod
  public void removeListeners(double count) {}

  private void emitFound(NsdServiceInfo service) {
    InetAddress host = service.getHost();
    if (host == null || service.getPort() < 1) return;
    String scheme = attribute(service, "scheme", "http");
    if (!"http".equals(scheme) && !"https".equals(scheme)) return;
    String address = host.getHostAddress();
    if (address == null || address.isEmpty()) return;
    int zone = address.indexOf('%');
    if (zone >= 0) address = address.substring(0, zone);
    if (address.indexOf(':') >= 0) address = "[" + address + "]";

    WritableMap payload = Arguments.createMap();
    payload.putString("id", service.getServiceName());
    payload.putString("name", service.getServiceName());
    payload.putString("url", scheme + "://" + address + ":" + service.getPort());
    payload.putString("protocol", attribute(service, "protocol", ""));
    emit("loomarrDiscoveryFound", payload);
  }

  private static String attribute(NsdServiceInfo service, String name, String fallback) {
    Map<String, byte[]> attributes = service.getAttributes();
    byte[] value = attributes.get(name);
    return value == null ? fallback : new String(value, StandardCharsets.UTF_8);
  }

  private void emitError(int code) {
    WritableMap payload = Arguments.createMap();
    payload.putInt("code", code);
    emit("loomarrDiscoveryError", payload);
  }

  private void emit(String event, WritableMap payload) {
    ReactApplicationContext context = getReactApplicationContext();
    if (!context.hasActiveReactInstance()) return;
    context.getJSModule(DeviceEventManagerModule.RCTDeviceEventEmitter.class).emit(event, payload);
  }
}

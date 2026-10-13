#import <React/RCTBridgeModule.h>
#import <React/RCTEventEmitter.h>

// Registers the Swift module under the same name and methods as the Android module.
@interface RCT_EXTERN_MODULE(LoomarrLanDiscovery, RCTEventEmitter)
RCT_EXTERN_METHOD(start)
RCT_EXTERN_METHOD(stop)
@end

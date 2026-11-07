#import <React/RCTBridgeModule.h>
#import <React/RCTEventEmitter.h>

// Registers the Swift MetricKit module with React Native.
@interface RCT_EXTERN_MODULE(LoomarrCrashReports, RCTEventEmitter)
RCT_EXTERN_METHOD(start)
RCT_EXTERN_METHOD(pending:(RCTPromiseResolveBlock)resolve rejecter:(RCTPromiseRejectBlock)reject)
RCT_EXTERN_METHOD(remove:(NSString *)id resolver:(RCTPromiseResolveBlock)resolve rejecter:(RCTPromiseRejectBlock)reject)
@end

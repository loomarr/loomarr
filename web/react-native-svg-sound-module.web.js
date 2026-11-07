// react-native-svg 15.16's touch mixin imports its Android touch-sound TurboModule through
// `TurboModuleRegistry`, which react-native-web does not export, so a production web bundle fails
// on the missing binding. The mixin only requires the module on Android; the web aliases it here.
export default undefined;

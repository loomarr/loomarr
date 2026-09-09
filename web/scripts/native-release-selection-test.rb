# Network-free regression for the actual consumer's source/prebuilt selection.
require 'json'
require File.realpath(File.join(__dir__, '../apps/mobile/node_modules/react-native/scripts/cocoapods/rndependencies.rb'))
require File.realpath(File.join(__dir__, '../apps/mobile/node_modules/react-native/scripts/cocoapods/rncore.rb'))
%w[RCT_DEPS_VERSION RCT_USE_LOCAL_RN_DEP RCT_TESTONLY_RNCORE_VERSION
   RCT_TESTONLY_RNCORE_TARBALL_PATH REACT_NATIVE_OVERRIDE_NIGHTLY_BUILD_VERSION RNTV_TESTONLY_LOCAL_RNCORE_REPOSITORY].each { |key| ENV.delete(key) }

failures = []
[
  [ReactNativeDependenciesUtils, 'RCT_USE_RN_DEP', :setup_react_native_dependencies, :build_react_native_deps_from_source, '0.86.2'],
  [ReactNativeCoreUtils, 'RCT_USE_PREBUILT_RNCORE', :setup_rncore, :build_rncore_from_source, '0.86.2-0']
].each do |consumer, variable, setup, from_source, version|
  consumer.define_singleton_method(:resolve_podspec_source) { {http: 'fixture-with-no-network'} }
  consumer.define_singleton_method(:rndeps_log) { |*| }
  consumer.define_singleton_method(:rncore_log) { |*| }
  [true, false].each do |available|
    consumer.class_variable_set(:@@react_native_version, '')
    consumer.class_variable_set(:@@use_nightly, false)
    consumer.define_singleton_method(:release_artifact_exists) { |_| available }
    consumer.define_singleton_method(:nightly_artifact_exists) { |_| raise 'nightly probe must not be reached' }
    ENV[variable] = '1'
    consumer.public_send(setup, 'fixture-package', version)
    failures << "#{consumer}: HTTP availability=#{available} changed pinned prebuilt selection" if consumer.public_send(from_source)
  end
  # Preserve explicit upstream development mode outside the strict verifier.
  consumer.class_variable_set(:@@react_native_version, '')
  ENV[variable] = '0'
  consumer.public_send(setup, 'fixture-package', version)
  failures << "#{consumer}: explicit source mode was lost" unless consumer.public_send(from_source)
end
if failures.any?
  warn failures.join("\n")
  exit 1
end
puts 'Pinned native dependency selections are independent of HTTP availability'

# Network-free regression of both installed Core and Dependencies acquisition consumers.
require 'tmpdir'
require 'pathname'
require File.realpath(File.join(__dir__, '../apps/mobile/node_modules/react-native/scripts/cocoapods/rndependencies.rb'))
require File.realpath(File.join(__dir__, '../apps/mobile/node_modules/react-native/scripts/cocoapods/rncore.rb'))
%w[RCT_SKIP_CACHES ENTERPRISE_REPOSITORY RNTV_TESTONLY_LOCAL_RNCORE_REPOSITORY].each { |key| ENV.delete(key) }

def check(condition, message)
  raise message unless condition
end

def rejects(message)
  begin
    yield
  rescue SystemExit => error
    check(!error.success?, "#{message}: exited successfully")
    return
  end
  raise "#{message}: unexpectedly accepted"
end

[
  [ReactNativeDependenciesUtils, :download_stable_rndeps, :podspec_source_download_prebuild_release_tarball, '0.86.2'],
  [ReactNativeCoreUtils, :download_stable_rncore, :podspec_source_download_prebuild_stable_tarball, '0.86.2-0']
].each do |consumer, download, source_method, version|
  Dir.mktmpdir('native contract#%') do |root|
    pods = File.join(root, 'pods')
    cache = File.join(root, 'cache')
    bytes = 'verified pinned binary archive'
    metadata = Digest::SHA1.hexdigest(bytes)
    requests = []
    fail_http = false
    consumer.define_singleton_method(:artifacts_dir) { pods }
    consumer.define_singleton_method(:rndeps_log) { |*| }
    consumer.define_singleton_method(:rncore_log) { |*| }
    ReactNativePodsUtils.define_singleton_method(:shared_cache_dir) { cache }
    ReactNativePinnedArtifacts.define_singleton_method(:fetch) do |url, destination|
      requests << url
      abort('fixture HTTP error') if fail_http
      File.write(destination, url.end_with?('.sha1') ? metadata : bytes)
    end
    acquire = -> { consumer.public_send(download, 'fixture-package', version, :debug) }
    path = acquire.call
    check(File.read(path) == bytes, "#{consumer}: new archive missing")
    requests.clear
    acquire.call
    check(requests.length == 1, "#{consumer}: Pods hit did not verify metadata")
    File.write(path, 'bad pods')
    requests.clear
    acquire.call
    check(requests.length == 1 && File.read(path) == bytes, "#{consumer}: shared cache repair failed")
    File.write(path, 'bad pods')
    File.write(File.join(cache, File.basename(path)), 'bad shared')
    requests.clear
    acquire.call
    check(requests.length == 2 && File.read(path) == bytes, "#{consumer}: corrupt caches were not reacquired")
    ['<html>error</html>', '', '0' * 39].each do |bad|
      metadata = bad
      requests.clear
      rejects('malformed metadata with cached bytes') { acquire.call }
      check(requests.length == 1, 'bad metadata caused alternate acquisition')
    end
    metadata = Digest::SHA1.hexdigest(bytes)
    fail_http = true
    rejects('HTTP outage with cached bytes') { acquire.call }
    fail_http = false
    ENV['RCT_SKIP_CACHES'] = '1'
    bytes = 'wrong downloaded bytes'
    rejects('cache bypass with wrong digest') { acquire.call }
    check(File.read(path) == 'verified pinned binary archive', 'failed download replaced valid archive')
    ENV.delete('RCT_SKIP_CACHES')
    bytes = 'verified pinned binary archive'
    consumer.class_variable_set(:@@react_native_path, 'fixture-package')
    consumer.class_variable_set(:@@react_native_version, version)
    consumer.class_variable_set(:@@build_from_source, false)
    consumer.class_variable_set(:@@download_dsyms, false) if consumer == ReactNativeCoreUtils
    requests.clear
    source = consumer.public_send(source_method)
    check(URI.decode_www_form_component(URI(source[:http]).path) == path, 'CocoaPods source does not round-trip reserved characters')
    check(source[:sha1] == metadata, 'CocoaPods lacks verified local digest')
    check(requests.any? { |url| url.include?('release.tar.gz') }, 'release configuration not acquired')
  end
end
puts 'Core and Dependencies strict acquisition, caches, and local CocoaPods contracts passed'

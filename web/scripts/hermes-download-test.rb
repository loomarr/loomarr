# Network-free regression of the actual pnpm-patched consumer, using only Ruby stdlib.
require 'tmpdir'
require 'json'
require File.realpath(File.join(__dir__, '../apps/mobile/node_modules/react-native/sdks/hermes-engine/hermes-utils.rb'))

%w[REACT_NATIVE_OVERRIDE_HERMES_DIR HERMES_ENGINE_TARBALL_PATH HERMES_COMMIT
   RCT_BUILD_HERMES_FROM_SOURCE ENTERPRISE_REPOSITORY RCT_SKIP_CACHES RNTV_TESTONLY_LOCAL_RNCORE_REPOSITORY].each { |key| ENV.delete(key) }

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

# Any release availability check would select snapshot/main on the broken version.
def release_artifact_exists(*)
  raise 'release selection must not probe HTTP availability'
end
check(hermes_source_type('250829098.0.16', 'unused') ==
      HermesEngineSourceType::DOWNLOAD_PREBUILD_RELEASE_TARBALL, 'pinned release was not selected')

# Keep the real curl wrapper under test. No curl process or network is allowed here.
class << Open3
  attr_accessor :fixture_status, :fixture_stderr, :observed_arguments
  alias_method :original_capture3, :capture3
  def capture3(*args)
    self.observed_arguments = args
    ['', fixture_stderr || '', Struct.new(:success?).new(fixture_status)]
  end
end
Open3.fixture_status = false
Open3.fixture_stderr = 'HTTP 503'
rejects('HTTP failure') { hermes_fetch('https://fixture.invalid/pinned', '/tmp/unused') }
args = Open3.observed_arguments
check(args.include?('--fail') && args.include?('--retry-all-errors'), 'HTTP failures are unchecked')
%w[--connect-timeout --max-time --retry --retry-max-time].each do |flag|
  check(args[args.index(flag).to_i + 1].to_i.positive?, "missing bound #{flag}")
end
Open3.fixture_status = true
hermes_fetch('https://fixture.invalid/pinned?literal=$(false)', '/tmp/unused')
check(Open3.observed_arguments.last.end_with?('$(false)'), 'URL was not passed as one literal argument')

class Fixture
  attr_accessor :metadata, :bytes, :fail_metadata, :fail_archive
  attr_reader :requests
  def initialize(root)
    @root = root
    @bytes = 'pinned archive bytes'
    @metadata = Digest::SHA1.hexdigest(@bytes)
    @requests = []
  end
  def artifacts_dir
    File.join(@root, 'pods')
  end
  def shared_cache_dir
    File.join(@root, 'cache')
  end
  def hermes_log(*)
  end
  def hermes_fetch(url, destination)
    @requests << url
    metadata = url.end_with?('.sha1')
    abort('fixture HTTP error') if metadata ? @fail_metadata : @fail_archive
    File.write(destination, metadata ? @metadata : @bytes)
  end
  def download
    download_hermes_tarball('unused', 'https://fixture.invalid/pinned', '250829098.0.16', :release)
  end
end

def fixture
  Dir.mktmpdir('hermes contract#%') { |root| yield Fixture.new(root) }
end

fixture do |consumer|
  path = consumer.download
  check(File.read(path) == consumer.bytes, 'valid download was not published')
  consumer.requests.clear
  check(consumer.download == path, 'valid Pods cache was not reused')
  check(consumer.requests.length == 1, 'cache bypassed metadata or downloaded again')
  File.write(path, 'corrupt Pods bytes')
  consumer.requests.clear
  consumer.download
  check(File.read(path) == consumer.bytes, 'shared cache did not repair corrupt Pods bytes')
  check(consumer.requests.length == 1, 'valid shared cache unnecessarily downloaded')
  File.write(path, 'corrupt Pods bytes')
  File.write(File.join(consumer.shared_cache_dir, File.basename(path)), 'corrupt shared bytes')
  consumer.requests.clear
  consumer.download
  check(consumer.requests.length == 2 && File.read(path) == consumer.bytes, 'both corrupt caches were not repaired')
  consumer.fail_metadata = true
  rejects('cached archive without checksum authority') { consumer.download }
end

['<html><head><link></head></html>', '', '0' * 39, 'g' * 40].each do |metadata|
  fixture do |consumer|
    consumer.metadata = metadata
    rejects('malformed checksum metadata') { consumer.download }
    check(consumer.requests.length == 1, 'bad metadata caused another artifact selection')
  end
end
fixture do |consumer|
  consumer.bytes = 'wrong downloaded bytes'
  rejects('wrong archive digest') { consumer.download }
  check(Dir.glob(File.join(consumer.artifacts_dir, '*')).empty?, 'wrong/partial archive was published')
end
fixture do |consumer|
  consumer.fail_archive = true
  rejects('archive HTTP error') { consumer.download }
  check(Dir.glob(File.join(consumer.artifacts_dir, '*')).empty?, 'failed archive was published')
end
fixture do |consumer|
  ENV['RCT_SKIP_CACHES'] = '1'
  consumer.bytes = 'wrong bytes with caches disabled'
  rejects('cache bypass must still verify integrity') { consumer.download }
ensure
  ENV.delete('RCT_SKIP_CACHES')
end
fixture do |consumer|
  source = consumer.send(:podspec_source_download_prebuild_release_tarball, 'unused', '250829098.0.16')
  check(source[:http].start_with?('file://'), 'CocoaPods would redownload unchecked remote bytes')
  check(URI.decode_www_form_component(URI(source[:http]).path) == File.join(consumer.artifacts_dir, 'hermes-ios-250829098.0.16-debug.tar.gz'), 'local CocoaPods URI does not round-trip spaces or reserved characters')
  check(source[:sha1] == consumer.metadata, 'CocoaPods source has no verified digest')
  check(consumer.requests.count { |url| !url.end_with?('.sha1') } == 2, 'debug and release were not both acquired')
end
puts 'Hermes pinned release, HTTP bounds, metadata, cache, integrity, and CocoaPods contracts passed'

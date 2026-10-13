require "json"

package = JSON.parse(File.read(File.join(__dir__, "package.json")))

# Autolinked into the iPhone app as a React Native community module (the podspec in the package root
# is what autolinking looks for). The Apple TV app excludes it; tvOS has no discovery yet.
Pod::Spec.new do |s|
  s.name         = "LoomarrLanDiscovery"
  s.version      = package["version"]
  s.summary      = "Finds a Loomarr server on the local network."
  s.homepage     = "https://github.com/loomarr/loomarr"
  s.license      = "MIT"
  s.authors      = "The Loomarr Authors"
  s.platforms    = { :ios => min_ios_version_supported }
  s.source       = { :git => "https://github.com/loomarr/loomarr.git" }
  s.source_files = "ios/**/*.{m,swift}"
  s.frameworks   = "Network"
  s.swift_version = "5.9"

  install_modules_dependencies(s)
end

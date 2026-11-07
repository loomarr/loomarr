require "json"

package = JSON.parse(File.read(File.join(__dir__, "package.json")))

# Autolinked into the iPhone app only. MetricKit's crash diagnostics are an iOS framework; the TV
# apps do not depend on this package, and Android's analogue (ApplicationExitInfo) is not built.
Pod::Spec.new do |s|
  s.name         = "LoomarrCrashReports"
  s.version      = package["version"]
  s.summary      = "Keeps MetricKit crash diagnostics until the app sends them to its Loomarr server."
  s.homepage     = "https://github.com/loomarr/loomarr"
  s.license      = "MIT"
  s.authors      = "The Loomarr Authors"
  s.platforms    = { :ios => min_ios_version_supported }
  s.source       = { :git => "https://github.com/loomarr/loomarr.git" }
  s.source_files = "ios/**/*.{m,swift}"
  s.frameworks   = "MetricKit"
  s.swift_version = "5.9"

  install_modules_dependencies(s)
end

class DatadogCli < Formula
  desc "Read-only CLI for querying Datadog APIs"
  homepage "https://gitea.roboalch.com/roboalchemist/datadog-cli"
  url "https://gitea.roboalch.com/roboalchemist/datadog-cli/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "PLACEHOLDER"
  license "MIT"

  depends_on "go" => :build

  def install
    ldflags = "-s -w -X main.version=#{version}"
    system "go", "build", *std_go_args(ldflags:)
  end

  test do
    assert_match "datadog-cli version", shell_output("#{bin}/datadog-cli --version")
  end
end

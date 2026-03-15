class Yelo < Formula
  desc "FTP-style CLI for Amazon S3 and Glacier"
  homepage "https://github.com/Dorky-Robot/yelo"
  version "0.6.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Dorky-Robot/yelo/releases/download/v0.6.0/yelo-aarch64-apple-darwin.tar.gz"
      sha256 "PLACEHOLDER"
    else
      url "https://github.com/Dorky-Robot/yelo/releases/download/v0.6.0/yelo-x86_64-apple-darwin.tar.gz"
      sha256 "PLACEHOLDER"
    end
  end

  on_linux do
    url "https://github.com/Dorky-Robot/yelo/releases/download/v0.6.0/yelo-x86_64-unknown-linux-gnu.tar.gz"
    sha256 "PLACEHOLDER"
  end

  def install
    bin.install "yelo"
  end

  test do
    assert_match "yelo #{version}", shell_output("#{bin}/yelo --version")
  end
end

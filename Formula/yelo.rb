class Yelo < Formula
  desc "FTP-style CLI for Amazon S3 and Glacier"
  homepage "https://github.com/Dorky-Robot/yelo"
  version "0.6.1"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Dorky-Robot/yelo/releases/download/v0.6.1/yelo-aarch64-apple-darwin.tar.gz"
      sha256 "94b58b3f644e47b7296389bda7bbbc2518cca2c2aed8df695651243bbdd7ec83"
    else
      url "https://github.com/Dorky-Robot/yelo/releases/download/v0.6.1/yelo-x86_64-apple-darwin.tar.gz"
      sha256 "ef8d7c1ae2f7d280f1fe408a9d2fa4dd59885363ee2ad1be3abfa16d4d11e13a"
    end
  end

  on_linux do
    url "https://github.com/Dorky-Robot/yelo/releases/download/v0.6.1/yelo-x86_64-unknown-linux-gnu.tar.gz"
    sha256 "a147401cf269a20ad73a0cb057bc0afb2f71d333b65b53a0163b2d8b2d9aa8ec"
  end

  def install
    bin.install "yelo"
  end

  test do
    assert_match "yelo #{version}", shell_output("#{bin}/yelo --version")
  end
end

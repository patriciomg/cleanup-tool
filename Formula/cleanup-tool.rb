class CleanupTool < Formula
  desc "Terminal-based disk cleanup tool for macOS"
  homepage "https://github.com/patriciomg/cleanup-tool"
  url "https://github.com/patriciomg/cleanup-tool/releases/download/v0.4.1/cleanup-tool-v0.4.1-darwin-universal.tar.gz"
  sha256 "7e04434b164f2c59fed17dcb8ed7488cf8327a16853680bf3a145dba59046a64"
  license "MIT"

  depends_on :macos

  def install
    bin.install "cleanup-tool"
  end

  test do
    system bin/"cleanup-tool", "-version"
  end
end

class CleanupTool < Formula
  desc "Terminal-based disk cleanup tool for macOS"
  homepage "https://github.com/patriciomg/cleanup-tool"
  url "https://github.com/patriciomg/cleanup-tool/releases/download/v0.3.0/cleanup-tool-v0.3.0-darwin-universal.tar.gz"
  sha256 "ca1eee152e75cb2905e3d95c67ea24739a5a6919971bf3ca1f570eaca9a6fe61"
  license "MIT"

  depends_on :macos

  def install
    bin.install "cleanup-tool"
  end

  test do
    system bin/"cleanup-tool", "-version"
  end
end

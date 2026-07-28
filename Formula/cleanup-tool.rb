class CleanupTool < Formula
  desc "Terminal-based disk cleanup tool for macOS"
  homepage "https://github.com/patriciomg/cleanup-tool"
  url "https://github.com/patriciomg/cleanup-tool/releases/download/v0.4.2/cleanup-tool-v0.4.2-darwin-universal.tar.gz"
  sha256 "b4aaf324c1addee7ede7de9be597d94fe75f664d91c1af9f418f1653baef20c1"
  license "MIT"

  depends_on macos: :big_sur

  def install
    bin.install "cleanup-tool"
  end

  test do
    system bin/"cleanup-tool", "-version"
  end
end

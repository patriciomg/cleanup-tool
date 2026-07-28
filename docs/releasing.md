# Releasing

The tag-triggered workflow in [`.github/workflows/release.yml`](../.github/workflows/release.yml) builds, signs, and publishes a GitHub Release whenever a `v*` tag is pushed.

> **Note:** `make release` must be run on macOS because the universal binary is created with `lipo`.

## Local release artifacts

```bash
# Build the universal binary, tarball, and checksums locally
cd cleanup-tool
make release

# Sign the artifacts with GPG (requires a configured GPG key).
# Optional: use GPG_KEY_ID to select a specific key.
make release-sign
# or: GPG_KEY_ID=YOUR_KEY_ID make release-sign

# Verify a signature
gpg --verify dist/checksums.txt.asc dist/checksums.txt
gpg --verify dist/cleanup-tool-<version>-darwin-universal.tar.gz.asc \
            dist/cleanup-tool-<version>-darwin-universal.tar.gz
```

## Publishing a release from GitHub Actions

1. **Generate a GPG key** (if you do not already have one):

   ```bash
   gpg --full-generate-key
   # Choose a secure key type, e.g., ED25519 or RSA/RSA 4096 bits.
   ```

2. **Export the private key** for GitHub Actions:

   ```bash
   gpg --list-secret-keys --keyid-format LONG
   # Use the long key ID from the output above
   gpg --armor --export-secret-keys <KEY_ID> > cleanup-tool-release.asc
   ```

3. **Add the secrets to your repository** on GitHub under **Settings → Secrets and variables → Actions**:

   - `GPG_PRIVATE_KEY` (required for signing): the contents of `cleanup-tool-release.asc`
   - `GPG_PASSPHRASE` (only if your key has a passphrase): the passphrase for that key
   - `GPG_KEY_ID` (optional): the long key ID or fingerprint to use if you have multiple signing keys

   If `GPG_PRIVATE_KEY` is not set, the workflow will still publish a release, but the assets will not be signed.

4. **Push a version tag** (`v*`):

   ```bash
   git tag -a v1.0.0 -m "Release v1.0.0"
   git push origin v1.0.0
   ```

5. The `release` workflow will:

   - Run tests and `go vet` on `macos-latest`
   - Build `cleanup-tool-darwin-universal` and the release tarball
   - Generate `checksums.txt`
   - Import the GPG key and run `make release-sign`
   - Verify the signatures
   - Create the GitHub Release and upload the tarball, checksum file, and `.asc` signatures

## Homebrew formula

The repository includes a native Homebrew formula at [`Formula/cleanup-tool.rb`](../Formula/cleanup-tool.rb). The release workflow automatically updates the formula's `url`, `version`, and `sha256` after a release is published, so no manual editing is required for normal releases.

Users can install or update the tool via Homebrew. Because the formula lives in this repository rather than a separate `homebrew-cleanup-tool` repo, users must tap the repository explicitly first:

```bash
brew tap patriciomg/cleanup-tool https://github.com/patriciomg/cleanup-tool.git
brew install patriciomg/cleanup-tool/cleanup-tool
```

> **Note:** If `main` is protected or the default `GITHUB_TOKEN` cannot push, the formula auto-bump step will fail. Add a repository secret named `TAP_GITHUB_TOKEN` (a Personal Access Token with `repo` scope). The release workflow uses it automatically when present.

### Homebrew tap trust

Homebrew 4.1+ treats third-party taps as untrusted by default and may refuse to install from them until the user explicitly trusts the tap or the specific formula. This is a Homebrew client-side security feature; the formula itself does not need to do anything special.

Users can trust the formula before installing:

```bash
brew tap patriciomg/cleanup-tool https://github.com/patriciomg/cleanup-tool.git
brew trust --formula patriciomg/cleanup-tool/cleanup-tool
brew install patriciomg/cleanup-tool/cleanup-tool
```

Or trust the entire tap once:

```bash
brew tap patriciomg/cleanup-tool https://github.com/patriciomg/cleanup-tool.git
brew trust patriciomg/cleanup-tool
brew install patriciomg/cleanup-tool/cleanup-tool
```

For the latest install instructions, including how to handle tap-trust prompts, see the [README.md](../README.md#install-with-homebrew-recommended).

### Testing the formula bump locally

You can dry-run the release tarball build and formula auto-bump without publishing anything:

```bash
./scripts/dry-run-formula-bump.sh [VERSION]
```

The script builds a local release tarball, updates `Formula/cleanup-tool.rb` with a local `file://` URL, runs `brew audit --new`, and restores the original formula afterwards.

## Verifying a published release

```bash
# Download the public key from the release author and import it
gpg --import <author-public-key.asc>

# Verify the checksum file signature
gpg --verify checksums.txt.asc checksums.txt

# Verify the tarball signature
gpg --verify cleanup-tool-<version>-darwin-universal.tar.gz.asc \
            cleanup-tool-<version>-darwin-universal.tar.gz
```

# Installing Anshin

How to put the app on your phone, check it, and keep it up to date. The Agent's install steps are in the
[README](../README.md#install-the-agent). This page covers the app only, for now.

> [!NOTE]
> No release carries an APK yet. [v0.1.0](https://github.com/fsrodrig/anshin/releases/tag/v0.1.0) has none, and
> adding this repository to Obtainium today finds nothing to install. The first release the
> [Release workflow](../.github/workflows/release.yml) cuts after the signing key exists will carry a signed APK. Until
> then, [build the debug APK from source](../README.md#install-the-app).

## The app, with Obtainium

[Obtainium](https://obtainium.imranr.dev/) installs and updates Android apps straight from a repository's GitHub
Releases, with no store in between.

1. Install Obtainium on the phone.
2. In Obtainium, choose **Add App** and enter `https://github.com/fsrodrig/anshin`.
3. Add it and install it. No filter is needed: every release carries exactly one APK, `anshin_X.Y.Z.apk`.

Obtainium offers an update when a new release is published. Android installs it over the old app, keeping your saved
Machines, only if the new APK is signed with the same certificate as the installed one and has a higher version.
Every release is signed with the one release key, so that holds from release to release.

## Check the APK

Obtainium installs what GitHub serves for the repository. Android itself checks only that an update carries the same
certificate as the app it replaces. To check the first install yourself, download the APK and `checksums.txt` from the
release to a computer and run these three checks, replacing `X.Y.Z`:

1. The checksum. `checksums.txt` lists every asset of the release, the APK included:

   ```sh
   sha256sum --check --ignore-missing checksums.txt
   ```

2. The attestation, which says this repository's Release workflow built the file. It needs the
   [GitHub CLI](https://cli.github.com/):

   ```sh
   gh attestation verify anshin_X.Y.Z.apk --repo fsrodrig/anshin
   ```

3. The certificate. `apksigner` comes with the Android SDK's build-tools. It must report one signer, and its
   `certificate SHA-256 digest` must equal the one line of
   [`deploy/android/release-certificate.sha256`](../deploy/android/release-certificate.sha256) on `main`, a file that
   changes only through a reviewed pull request:

   ```sh
   apksigner verify --verbose --print-certs --min-sdk-version 26 anshin_X.Y.Z.apk
   ```

   From a checkout of this repository, `scripts/apk-verify.sh anshin_X.Y.Z.apk deploy/android/release-certificate.sha256`
   does the comparison for you. It exits 0 only for a valid signature by exactly that certificate, on a 16 KB-aligned APK.

The same checks are how the Release workflow decides to publish: it refuses any APK that isn't signed by the pinned
certificate. The key is described in [maintaining.md](maintaining.md#the-apk-signing-key).

## Switching from a debug or self-built APK

An APK you built yourself is signed with a different key, so Android refuses to update it with the release APK. Remove
the old one first:

1. Uninstall Anshin from the phone. **Your saved Machines live only on the phone, so you'll add them again.**
2. Install the release APK, through Obtainium or by hand.
3. Add your Machines again ([README § Add your first Machine](../README.md#add-your-first-machine)).

Settings → Version tells the two apart: a release APK shows its number, and a build without a version, such as one you
built from source, shows "Development build".

## Updating and uninstalling

Obtainium updates the app when it finds a newer release. To uninstall, remove Anshin like any other app; it keeps
nothing anywhere else. This section, and the Agent's, are completed with the Agent's installation in
[#20](https://github.com/fsrodrig/anshin/issues/20).

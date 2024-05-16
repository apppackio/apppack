# AppPack CLI

![goreleaser](https://github.com/apppackio/apppack/workflows/goreleaser/badge.svg)

The command line interface for [AppPack.io](https://apppack.io).

**[Documentation](https://docs.apppack.io)**

## Release Process Overview

The release process is designed to ensure that all changes are accurately tracked, versioned, and distributed according to [`Semantic Versioning (SemVer)` standards](https://semver.org/). Here's a step-by-step guide to releasing a new version of AppPack:

### Documenting Changes

* Update `CHANGELOG.md`: Enhance the `CHANGELOG.md` file with all relevant changes associated with the upcoming release, including the release tag and the date. This step is crucial for maintaining a clear history of updates and enhancements.

* **Semantic Versioning (SemVer)**: Adhere to [SemVer guidelines](https://semver.org/) when incrementing version numbers.

### Automated Release with GoReleaser

* **Commit and Tag**: Commit the changes to your local repository, including a tag with the new version number that corresponds to the release you've documented in the CHANGELOG.

* **GoReleaser Configuration**:  Filters are applied in GoReleaser configuration
that ensures that the changelog only includes changes that affect the functionality of the software, thereby adhering to SemVer practices.

* **Distribution**: Once you push the commit and tag to the repository, `GoReleaser` will take over through Github Actions, creating a new release with the version number specified in your tag. It will also compile the latest code, create OS-specific binaries, and upload these artifacts to the release assets on GitHub.

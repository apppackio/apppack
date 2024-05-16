# AppPack CLI

![goreleaser](https://github.com/apppackio/apppack/workflows/goreleaser/badge.svg)

The command line interface for [AppPack.io](https://apppack.io).

**[End user documentation](https://docs.apppack.io)**

---

## Development

When writing commit logs think of the impact to the end user. Commits are listed in the release notes on GitHub with the exception of [the prefixes defined in .goreleaser.yml](https://github.com/apppackio/apppack/blob/main/.goreleaser.yml#L57-L62). Use those whenever you're making changes which don't effect the end-user.

## Release Process Overview

Here's a step-by-step guide to releasing a new version of AppPack:

### Prepping for release

* Update CHANGELOG.md with all relevant user-facing changes associated with the upcoming release, including the release tag and the date.
* Use [SemVer guidelines](https://semver.org/) when incrementing version numbers.

### Automated Release with GoReleaser

* Push you changes to `main`
* Tag the commit with the version number prefixed by `v`. For example, to tag 9.7.5, run `git tag -s v9.7.5`
* `git push --tag`
* GoReleaser will take over through Github Actions, creating a new release with the version number specified in your tag. It will also compile the latest code, create OS-specific binaries, and upload these artifacts to the release assets on GitHub.

### Update docs

* Run the [docs workflow](https://github.com/apppackio/apppack-docs/actions/workflows/ci_cd.yml) to update the docs once GoReleaser completes. Make sure to run workflow from `deploy/prod` branch to make it live.

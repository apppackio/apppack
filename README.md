# AppPack CLI

![goreleaser](https://github.com/apppackio/apppack/workflows/goreleaser/badge.svg)

The command line interface for [AppPack.io](https://apppack.io).

**[Documentation](https://docs.apppack.io)**

## Release Process Overview

To manage releases effectively, we follow a structured procedure that ensures all changes are documented and distributed as intended. Below are the steps involved in releasing a new version of our software:

**Document Changes**: Update the `CHANGELOG.md` file with a comprehensive list of changes that occurred since the last release. Include the release tag and the date to provide context for when these changes were implemented.

**Version Control**: Commit these changes to your local repository, ensuring that the new version number is properly tagged alongside the commit that introduced this change. This tag will be used to reference the specific release in question.

**Automated Release and Distribution**: Upon pushing the commit and tag to our repository, our automated Github Actions workflow will take over. It will generate a new release using the version number specified in the tag. Additionally, it will trigger the go releaser tool, which will compile and package the software for the supported operating systems and automatically upload the binaries to our release assets on GitHub.

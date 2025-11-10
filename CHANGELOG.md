# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

* `build start` command now accepts optional `--ref` flag to build from specific git references (branches, tags, or commit hashes).
* `modify app` command to update some parameters of application/pipeline stacks.

### Changed

* Updated to Go 1.25.4 and refreshed dependencies.

### Fixed

* Fixed issue where a stack update could revert unexpected parameters to template defaults.

## [4.6.7] - 2025-08-14

### Fixed
* Fix for a change in heroku/builder:24 causing HOME=/root resulting in various permissions errors.

## [4.6.6] - 2025-03-31

### Fixed

* Handle and return a clear error for empty CloudFormation change sets instead of `ResourceNotReady` error.
* Raise an exception when no scheduled tasks are available for deletion.

## [4.6.5] - 2025-03-06

### Fixed

* Properly ignore errors on region deletion if the legacy `dockerhub-access-token` is not found.

## [4.6.4] - 2025-03-05

### Removed

* Region creation no longer requires Docker Hub credentials. Existing apps must be upgraded for compatibility.

### Fixed

* Resizing a non-existent service in an undeployed app no longer causes an error.
* Network issues are now displayed separately from authentication errors. Previously, network failures during authentication token refresh were incorrectly shown as authentication errors.

## [4.6.3] - 2024-11-04

### Fixed

* Excluded `limitless` suffix from Aurora PostgreSQL version selection to ensure compatibility with general-purpose instance types.
* Verify existence of review app before resizing process with `ps resize`.

## [4.6.2] - 2024-09-03

### Fixed

* Allow `apppack build` commands to work for non-pipeline apps.

## [4.6.1] - 2024-08-28

### Fixed

* Revert the ability to provide `APPPACK_ACCOUNT` for multiple accounts.

## [4.6.0] - 2024-08-28

### Fixed

* Prevent `Ctrl+C` from exiting the remote shell session prematurely.

### Changed

* Limits the number of custom domains to 4.
* `ps resize` raises a warning for non-existent service.
* `reviewapps` cmd optionally accepts `-c`/ `account` flag.
* Implemented a check that throws an error if neither the `-c` flag nor the `APPPACK_ACCOUNT` environment variable is set and the user has multiple accounts. This ensures that users specify an account explicitly to avoid ambiguity.

## [4.5.0] - 2024-05-16

### Changed

* Integrates AWS Session manager directly.
* Updated dependencies.
* Improve error handling for user info API call.
* Improve error message when user needs admin access.

## [4.4.0] - 2023-03-25

### Changed

* Upgraded to Go 1.22 and updated dependencies

### Fixed

* Allow `ps` to work on clusters with large active task counts

## [4.3.0] - 2023-03-27

### Added

* CLI automatically checks for updates
* Report crashes to Sentry

### Fixed

* `upgrade region` now works as expected
* `create database` only shows valid instance sizes

### Changed

* `shell` command no longer requires `wc` in the remote container

## [4.2.0] - 2023-03-03

### Added

* `shell` command now supports Dockerfile builds
* `ps resize` can now be run on a pipeline to apply to all review apps
* `create custom-domain` now supports wildcard domains
* `create redis` now lists available instance sizes

### Fixed

* `upgrade pipeline` works with latest changes to stack

## [4.1.0] - 2023-01-10

### Added

* Experimental `dash` command for viewing app metric graphs

### Changed

* Upgraded to Go 1.19 and updated dependencies

### Fixed

* Prevent using an invalid index when deleting scheduled tasks which led to panic

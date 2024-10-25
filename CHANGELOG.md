# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).


## [Unreleased] - TBD

### Fixed

* Verify existence of review app before resizing process with `ps resize`.

### Removed

* Region creation no longer requires Docker Hub credentials

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

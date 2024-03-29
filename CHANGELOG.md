# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

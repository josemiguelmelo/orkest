# Releases

The `Release` GitHub Actions workflow starts after each push to `main` (including
merged pull requests). It calculates the next stable semantic version from
commit messages since the latest `vMAJOR.MINOR.PATCH` tag:

- `BREAKING CHANGE:` or a `!` after the commit type increments the major version.
- `feat:` or `feature:` increments the minor version.
- Other commits increment the patch version.

The calculated release waits at the `release` GitHub Environment. Configure that
environment in repository settings with required reviewers to make publishing a
manual approval step. After approval, the workflow publishes the server image,
agentctl base image, and all five runtime images to GHCR, tagged with both
`vMAJOR.MINOR.PATCH` and `latest`, then creates the matching GitHub Release/tag.
The workflow's `GITHUB_TOKEN` needs repository contents write and package write
permissions, which are declared in the workflow.

The workflow can also be started manually with `workflow_dispatch`; it only
prepares a release for `main`. A release with no commits since the latest version
tag fails during preparation.

---
name: New release
about: Propose a new release
title: Release v0.0.0
labels: ''
assignees: ''

---

## Release Process
<!--
If making adjustments to the checklist please also file a PR against this issue
template (.github/ISSUE_TEMPLATE/new-release.md) to incorporate the changes for
future releases.
-->
- [ ] In the issue description, add a changelog section, describing changes since the last release.
- Local release preparations
  - [ ] Perform mandatory internal release checks and preparations.
  - [ ] Run `make release-tests` to run an extended set of tests prior to a release.
  - [ ] Sync/tidy up dependencies.
    - [ ] Run `go mod tidy`.
    - [ ] Run `git commit -m 'go.mod,go.sum: update dependencies.' go.{mod,sum}`, if necessary.
  - [ ] Run `git tag -a -m "CRI Resource Manager release $VERSION" $VERSION`.
- Publishing
  - [ ] Push the tag with `git push $VERSION`. This will automatically build container images and release assets and upload the release assets to a new draft release,
  - [ ] Check that release assets were created for the tag
    - Container images are published
      - https://hub.docker.com/r/intel/cri-resmgr-agent/tags
      - https://hub.docker.com/r/intel/cri-resmgr-webhook/tags
    - Release assets are uploaded to the draft release
      - RPM packages
      - DEB package
      - Binary tarball
      - Source+dependencies tarball (vendored dist)
  - [ ] Update the automatically created draft release corresponding to the tag.
    - [ ] Write the change log to the release.
    - [ ] Mark the release as a non-production pre-release if necessary.
    - [ ] Save as draft.
  - [ ] Get the change log OK'd by other maintainers.
  - [ ] Publish the draft as a release.
  - [ ] Add a link to the tagged release in this issue.
- [ ] Close this issue.


## Changelog
<!--
Capture changes since the last release here.
For major releases have separate sections for major changes and a more detailed changelog.
For minor releases list the most important bug fixes and other improvements.
-->
### Major changes

### Detailed changelog

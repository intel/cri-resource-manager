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
  - [ ] Create source+dependencies tarball with `make vendored-dist`.
  - [ ] Create binary tarball with `make cross-tar`.
  - [ ] Build RPM packages with `make cross-rpm`.
  - [ ] Build DEB packages with `make cross-deb`.
  - [ ] Build container images with `make images`.
- Final verification of artefacts
  - [ ] Verify the installation of binary packages.
  - [ ] Verify runnability of container images.
- Publishing
  - [ ] Push the tag with `git push $VERSION`.
  - [ ] Create a [new *draft* release](https://github.com/intel/cri-resource-manager/releases/new) corresponding to the tag.
    - [ ] Upload all artefacts to the release.
    - [ ] Write the change log to the release.
    - [ ] Mark the release as a non-production pre-release if necessary.
    - [ ] Save as draft.
  - [ ] Check that new container images are published for the tag.
    - https://hub.docker.com/r/intel/cri-resmgr-agent/tags
    - https://hub.docker.com/r/intel/cri-resmgr-webhook/tags
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

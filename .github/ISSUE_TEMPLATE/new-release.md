---
name: New release
about: Propose a new release
title: Release v0.0.0
labels: ''
assignees: ''

---

## Release Process
i<!--
If making adjustments to the checklist please also file a PR against this issue
template (.github/ISSUE_TEMPLATE/new-release.md) to incorporate the changes for
future releases.
-->
- [ ] In the issue description, add a changelog section, describing changes since the last release.
- Local release preparations
  - [ ] Mandatory internal release checks and preparations.
  - [ ] Run `make release-tests` to run an extended set of tests prior to a release
  - [ ] Run `git tag -a -m "CRI Resource Manager release $VERSION" $VERSION`.
  - [ ] Create src tarballs with `make vendored-dist`
  - [ ] Build RPM packages with `make cross-rpm`
  - [ ] Build DEB packages with `make cross-deb`
  - [ ] Upload all artefacts to Github release page.
  - [ ] Build container images with `make images`
- Final verification of artefacts
  - [ ] Verify the installation of binary packages
  - [ ] Verify runnability of container images
- Publishing
  - [ ] Push the tag with `git push $VERSION`
  - [ ] Check that container images are published
    - https://hub.docker.com/r/intel/cri-resmgr-agent/tags
    - https://hub.docker.com/r/intel/cri-resmgr-webhook/tags
  - [ ] Write the change log into the
  [Github release info](https://github.com/intel/cri-resource-manager/releases).
  - [ ] Add a link to the tagged release in this issue.
- [ ] Close this issue.


## Changelog
<!--
Capture changes since the last release here.
For major releases have separate sections for major changes and a more detailed changelog.
-->
### Major changes

### Detailed changelog

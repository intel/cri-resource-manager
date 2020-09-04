# Release Process

The process to release a new version of cri-resource-manager is:

- [ ] File [a new issue](https://github.com/intel/cri-resource-manager/issues/new)
  to propose a new release. Copy this checklist into the issue description.
- [ ] In the issue description, add a changelog section, describing changes
  since the last release.
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
  - [ ] Push container images *to where*???
  - [ ] Write the change log into the
  [Github release info](https://github.com/intel/cri-resource-manager/releases).
  - [ ] Add a link to the tagged release in this issue.
- [ ] Close this issue.

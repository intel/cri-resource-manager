# Go compiler/toolchain and extra related binaries we ues/need.
GO_CMD    := go
GO_BUILD  := $(GO_CMD) build
GO_GEN    := $(GO_CMD) generate -x
GO_FMT    := gofmt
GO_CYCLO  := gocyclo
GO_LINT   := golint
GO_CILINT := golangci-lint

# TEST_TAGS is the set of extra build tags passed for tests.
# We disable AVX collector for tests by default.
TEST_TAGS := -tags noavx
GO_TEST   := $(GO_CMD) test $(TEST_TAGS)

# Disable some golangci_lint checkers for now until we have an more acceptable baseline...
GO_CILINT_CHECKERS := -D unused,staticcheck,errcheck,deadcode,structcheck,gosimple -E golint,gofmt

# Protoc compiler and protobuf definitions we might need to recompile.
PROTOC    := $(shell command -v protoc || echo 'WARNING: no protoc, cannot run protoc ')
PROTOBUFS  = $(shell find cmd pkg -name \*.proto)
PROTOCODE := $(patsubst %.proto,%.pb.go,$(PROTOBUFS))

CLANG := clang
KERNEL_VERSION ?= $(shell uname -r)
KERNEL_SRC_DIR ?= /lib/modules/$(KERNEL_VERSION)/source
KERNEL_BUILD_DIR ?= /lib/modules/$(KERNEL_VERSION)/build

# Binaries and directories for installation.
PREFIX     ?= /usr
BINDIR     ?= $(PREFIX)/bin
UNITDIR    ?= $(PREFIX)/lib/systemd/system
SYSCONFDIR ?= /etc
INSTALL    := install

# Directories (in cmd) with go code we'll want to build and install.
BUILD_DIRS = $(shell find cmd -name \*.go | sed 's:cmd/::g;s:/.*::g' | uniq)
BUILD_BINS = $(foreach dir,$(BUILD_DIRS),bin/$(dir))

# Directories (in cmd) with go code we'll want to create Docker images from.
IMAGE_DIRS  = $(shell find cmd -name Dockerfile | sed 's:cmd/::g;s:/.*::g' | uniq)
IMAGE_TAG  := testing
IMAGE_REPO := ""

# List of our active go modules.
GO_MODULES = $(shell $(GO_CMD) list ./... | grep -v vendor/)
GO_PKG_SRC = $(shell find pkg -name \*.go)

# List of visualizer collateral files to go generate.
UI_ASSETS := $(shell for i in pkg/cri/resource-manager/visualizer/*; do \
        if [ -d "$$i" -a -e "$$i/assets_generate.go" ]; then \
            echo $$i/assets_vfsdata.go; \
        fi; \
    done)

# Git (tagged) version and revisions we'll use to linker-tag our binaries with.
GIT_ID   = scripts/build/git-id
BUILD_ID = "$(shell head -c20 /dev/urandom | od -An -tx1 | tr -d ' \n')"
LDFLAGS  = -ldflags "-X=github.com/intel/cri-resource-manager/pkg/version.Version=$$gitversion \
                     -X=github.com/intel/cri-resource-manager/pkg/version.Build=$$gitbuildid \
                     -B 0x$(BUILD_ID)"

# Build non-optimized version for debugging on make DEBUG=1.
DEBUG ?= 0
ifeq ($(DEBUG),1)
    GCFLAGS=-gcflags "all=-N -l"
else
    GCFLAGS=
endif

# Tarball timestamp, tar-related commands.
TAR          := tar
TIME_STAMP   := $(shell date +%Y%m%d)
TAR_UPDATE   := $(TAR) -uf
TAR_COMPRESS := bzip2
TAR_SUFFIX   := bz2

# Metadata for packages, changelog, etc.
USER_NAME  ?= $(shell git config user.name)
USER_EMAIL ?= $(shell git config user.email)
BUILD_DATE ?= $(shell date -R)

HOST_DISTRO ?= $(shell [ -f /etc/os-release ] && eval `cat /etc/os-release`; echo $$ID)
DEB_DISTRO  ?= ubuntu

# RPM spec files we might want to generate.
SPEC_FILES = $(shell find packaging -name \*.spec.in | sed 's/.spec.in/.spec/g' | uniq)

# Systemd collateral.
SYSTEMD_DIRS = $(shell find cmd -name \*.service -o -name \*.socket | sed 's:cmd/::g;s:/.*::g'|uniq)
SYSCONF_DIRS = $(shell find cmd -name \*.sysconf | sed 's:cmd/::g;s:/.*::g' | uniq)

# Extra options to pass to docker (for instance --network host).
DOCKER_OPTIONS =

# Docker boilerplate/commands to build debian/ubuntu packages.
DOCKER_DEB_BUILD := mkdir -p /build && cd /build && \
    git clone /input/cri-resource-manager && cd /build/cri-resource-manager && \
    make BUILD_DIRS=cri-resmgr deb

# Where to leave built packages, if/when we build them in containers.
PACKAGES_DIR = packages

# Be quiet by default but let folks override it with Q= on the command line.
Q := @

# Default target: just build everything.
all: build

#
# Generic targets: build, install, clean, build images.
#

build: $(BUILD_BINS)

install: $(BUILD_BINS) $(foreach dir,$(BUILD_DIRS),install-bin-$(dir)) \
    $(foreach dir,$(BUILD_DIRS),install-systemd-$(dir)) \
    $(foreach dir,$(BUILD_DIRS),install-sysconf-$(dir))

clean: $(foreach dir,$(BUILD_DIRS),clean-$(dir)) clean-spec clean-ui-assets

images: $(foreach dir,$(IMAGE_DIRS),image-$(dir))

#
# Rules for building and installing binaries, or building docker images, and cleaning up.
#

KERNEL_INCLUDE_DIRS = /include \
                      /include/uapi \
                      /include/generated/uapi \
                      /arch/x86/include \
                      /arch/x86/include/uapi \
                      /arch/x86/include/generated/uapi

KERNEL_INCLUDES := $(strip $(foreach kernel_dir,$(KERNEL_SRC_DIR) $(KERNEL_BUILD_DIR),$(addprefix -I,$(wildcard $(addprefix $(kernel_dir),$(KERNEL_INCLUDE_DIRS))))))

libexec/%.o: elf/%.c
	$(Q)if [ -z "$(KERNEL_INCLUDES)" ]; then echo "Cannot build $@: invalid KERNEL_SRC_DIR=$(KERNEL_SRC_DIR)"; exit 1; fi
	$(Q)echo "Building $@"
	$(Q)mkdir -p libexec
	$(Q)$(CLANG) -nostdinc -D __KERNEL__ $(KERNEL_INCLUDES) -O2 -Wall -target bpf -c $< -o $@

bin/%:
	$(Q)bin=$(notdir $@); src=cmd/$$bin; \
	eval `$(GIT_ID)`; \
	echo "Building $@ (version $$gitversion, build $$gitbuildid)..."; \
	mkdir -p bin && \
	cd $$src && \
	    $(GO_BUILD) $(BUILD_TAGS) $(LDFLAGS) $(GCFLAGS) -o ../../bin/$$bin

install-bin-%: bin/%
	$(Q)bin=$(patsubst install-bin-%,%,$@); dir=cmd/$$bin; \
	echo "Installing $$bin in $(DESTDIR)$(BINDIR)..."; \
	$(INSTALL) -d $(DESTDIR)$(BINDIR) && \
	$(INSTALL) -m 0755 -t $(DESTDIR)$(BINDIR) bin/$$bin; \

install-systemd-%:
	$(Q)bin=$(patsubst install-systemd-%,%,$@); dir=cmd/$$bin; \
	echo "Installing systemd collateral for $$bin..."; \
	$(INSTALL) -d $(DESTDIR)$(UNITDIR) && \
	for f in $(shell find $(dir) -name \*.service -o -name \*.socket); do \
	    echo "  $$f in $(DESTDIR)$(UNITDIR)..."; \
	    $(INSTALL) -m 0644 -t $(DESTDIR)$(UNITDIR) $$f; \
	done

install-sysconf-%:
	$(Q)bin=$(patsubst install-sysconf-%,%,$@); dir=cmd/$$bin; \
	echo "Installing sysconf collateral for $$bin..."; \
	$(INSTALL) -d $(DESTDIR)$(SYSCONFDIR)/sysconfig && \
	for f in $(shell find $(dir) -name \*.sysconf); do \
	    echo "  $$f in $(DESTDIR)$(SYSCONFDIR)/sysconfig..."; \
	    df=$${f##*/}; df=$${df%.sysconf}; \
	    $(INSTALL) -m 0644 -T $$f $(DESTDIR)$(SYSCONFDIR)/sysconfig/$$df; \
	done

clean-%:
	$(Q)bin=$(patsubst clean-%,%,$@); src=cmd/$$bin; \
	echo "Cleaning up $$bin..."; \
	rm -f bin/$$bin

image-%:
	$(Q)bin=$(patsubst image-%,%,$@); src=cmd/$$bin; \
	echo "Building docker image for $$src"; \
	    buildopts="--image cri-resmgr-$${bin#cri-resmgr-}"; \
	    if [ -n "$(IMAGE_TAG)" ]; then \
		buildopts="$$buildopts --tag $(IMAGE_TAG)"; \
	    fi; \
	    if [ -n "$(IMAGE_REPO)" ]; then \
	        buildopts="$$buildopts --publish $(IMAGE_REPO)"; \
	    fi; \
	    echo "Vendoring dependencies..."; \
	    go mod vendor && \
	        scripts/build/docker-build $(DOCKER_OPTIONS) $$buildopts $$src; \
	        rc=$$?; \
	    rm -fr vendor; \
	    exit $$rc

#
# Rules for format checking, various code quality and complexity checks and measures.
#

format:
	$(Q)report=`$(GO_FMT) -s -d -w $$(find cmd pkg test/functional -name \*.go)`; \
	if [ -n "$$report" ]; then \
	    echo "$$report"; \
	    exit 1; \
	fi

vet:
	$(Q)$(GO_CMD) vet $(GO_MODULES)

cyclomatic-check:
	$(Q)report=`$(GO_CYCLO) -over 15 cmd pkg`; \
	if [ -n "$$report" ]; then \
	    echo "Complexity is over 15 in"; \
	    echo "$$report"; \
	    exit 1; \
	fi

lint:
	$(Q)rc=0; \
	for f in $$(find -name \*.go | grep -v \.\/vendor); do \
	    $(GO_LINT) -set_exit_status $$f || rc=1; \
	done; \
	exit $$rc

golangci-lint:
	$(Q)$(GO_CILINT) run $(GO_CILINT_CHECKERS)


#
# Rules for running unit/module tests.
#

test:
ifndef WHAT
	$(Q)$(GO_TEST) -race -coverprofile=coverage.txt -covermode=atomic \
	    $(GO_MODULES)
else
	$(Q)cd $(WHAT) && \
            $(GO_TEST) -v -cover -coverprofile cover.out || rc=1; \
            $(GO_CMD) tool cover -html=cover.out -o coverage.html; \
            rm cover.out; \
            echo "Coverage report: file://$$(realpath coverage.html)"; \
            exit $$rc
endif

#
# Rule for building dist-tarballs, SPEC files, RPMs, debian collateral, deb's.
#

dist:
	$(Q)eval `$(GIT_ID) .` && \
	tardir=cri-resource-manager-$$gitversion; \
	tarball=cri-resource-manager-$$gitversion.tar; \
	echo "Creating $$tarball.$(TAR_SUFFIX)..."; \
	rm -fr $$tardir $$tarball* && \
	git archive --format=tar --prefix=$$tardir/ HEAD > $$tarball && \
	mkdir -p $$tardir && cp git-version git-buildid $$tardir && \
	$(TAR_UPDATE) $$tarball $$tardir && \
	$(TAR_COMPRESS) $$tarball && \
	rm -fr $$tardir

spec: clean-spec $(SPEC_FILES)

%.spec:
	$(Q)echo "Generating RPM spec file $@..."; \
	eval `$(GIT_ID) .` && \
	cp $@.in $@ && \
	sed -E -i -e "s/__VERSION__/$$rpmversion/g"    \
	          -e "s/__TARVERSION__/$$gitversion/g" \
	          -e "s/__BUILDID__/$$gitbuildid/g" $@

clean-spec:
	$(Q)rm -f $(SPEC_FILES)

rpm: spec dist
	mkdir -p ~/rpmbuild/{SOURCES,SPECS} && \
	cp packaging/rpm/cri-resource-manager.spec ~/rpmbuild/SPECS && \
	cp cri-resource-manager*.tar.bz2 ~/rpmbuild/SOURCES && \
	rpmbuild -bb ~/rpmbuild/SPECS/cri-resource-manager.spec

src.rpm source-rpm: spec dist
	mkdir -p ~/rpmbuild/{SOURCES,SPECS} && \
	cp packaging/rpm/cri-resource-manager.spec ~/rpmbuild/SPECS && \
	cp cri-resource-manager*.tar.bz2 ~/rpmbuild/SOURCES && \
	rpmbuild -bs ~/rpmbuild/SPECS/cri-resource-manager.spec

debian/%: packaging/deb.in/%
	$(Q)echo "Generating debian packaging file $@..."; \
	mkdir -p debian; \
	eval `$(GIT_ID) .` && \
	tarball=cri-resource-manager-$$gitversion.tar && \
	cp $< $@ && \
	sed -E -i -e "s/__PACKAGE__/cri-resource-manager/g" \
	          -e "s/__TARBALL__/$$tarball/g"            \
	          -e "s/__VERSION__/$$debversion/g"         \
	          -e "s/__AUTHOR__/$(USER_NAME)/g"          \
	          -e "s/__EMAIL__/$(USER_EMAIL)/g"          \
	          -e "s/__DATE__/$(BUILD_DATE)/g"           \
	          -e "s/__BUILD_DIRS__/$(BUILD_DIRS)/g" $@

clean-deb:
	$(Q)rm -f debian

deb: debian/changelog debian/control debian/rules debian/compat dist
	$(Q)if [ -z "$$BUILD_CONTAINER" -a "$(HOST_DISTRO)" != "$(DEB_DISTRO)" ]; then \
	    $(MAKE) deb-docker-$(DEB_DISTRO); \
	    exit $$?; \
	fi; \
	dpkg-buildpackage -uc

deb-docker-%: docker/%-build
	$(Q)distro=$(patsubst deb-docker-%,%,$@); \
	builddir=build/docker/$$distro; \
	outdir=$(PACKAGES_DIR)/$$distro; \
	echo "Docker cross-building $$distro packages..."; \
	mkdir -p $(PACKAGES_DIR)/$$distro && \
	rm -fr $$builddir && mkdir -p $$builddir && \
	docker run --rm -ti $(DOCKER_OPTIONS) --user $(shell echo $$USER) \
	    --env USER_NAME="$(USER_NAME)" --env USER_EMAIL=$(USER_EMAIL) \
	    -v $$(pwd):/input/cri-resource-manager \
	    -v $$(pwd)/$$builddir:/build \
	    -v $$(pwd)/$$outdir:/output \
	    $$distro-build /bin/bash -c "export DEB_DISTRO=$$distro; $(DOCKER_DEB_BUILD)" && \
	cp $$builddir/cri-resource-manager*.* $$outdir && \
	rm -fr $$builddir

ubuntu-packages:
	$(MAKE) DEB_DISTRO=ubuntu deb

debian-packages:
	$(MAKE) DEB_DISTRO=debian deb

# Build a docker image (for distro cross-building).
docker/%: dockerfiles/Dockerfile.%
	$(Q)img=$(patsubst docker/%,%,$@); \
	docker rm $$img || : && \
	echo "Building cross-build docker image $$img..."; \
	scripts/build/docker-build-image $$img --container $(DOCKER_OPTIONS)

# Rule for recompiling a changed protobuf.
%.pb.go: %.proto
	$(Q)echo "Generating go code ($@) for updated protobuf $<..."; \
	$(PROTOC) -I . $< --go_out=plugins=grpc:.

# Rule for installing in-repo git hooks.
install-git-hooks:
	$(Q)if [ -d .git -a ! -e .git-hooks.redirected ]; then \
	    echo -n "Redirecting git hooks to .githooks..."; \
	    git config core.hookspath .githooks && \
	    touch .git-hooks.redirected && \
	    echo "done."; \
	fi

#
# go dependencies for our binaries (careful with that axe, Eugene...)
#

bin/cri-resmgr: $(wildcard cmd/cri-resmgr/*.go) $(UI_ASSETS) \
    $(shell for dir in \
                  $(shell go list -f '{{ join .Deps  "\n"}}' ./cmd/cri-resmgr/... | \
                          grep cri-resource-manager/pkg/ | \
                          sed 's#github.com/intel/cri-resource-manager/##g'); do \
                find $$dir -name \*.go; \
            done | sort | uniq)

bin/cri-resmgr-agent: $(wildcard cmd/cri-resmgr-agent/*.go) \
    $(shell for dir in \
                  $(shell go list -f '{{ join .Deps  "\n"}}' ./cmd/cri-resmgr-agent/... | \
                          grep cri-resource-manager/pkg/ | \
                          sed 's#github.com/intel/cri-resource-manager/##g'); do \
                find $$dir -name \*.go; \
            done | sort | uniq)

bin/webhook: $(wildcard cmd/webhook/*.go) \
    $(shell for dir in \
                  $(shell go list -f '{{ join .Deps  "\n"}}' ./cmd/webhook/... | \
                          grep cri-resource-manager/pkg/ | \
                          sed 's#github.com/intel/cri-resource-manager/##g'); do \
                find $$dir -name \*.go; \
            done | sort | uniq)

#
# rules to run go generators
#

#
# %_vfsdata.go should also depend on the collateral content.
#
# We'd need a correctly expanding/working equivalent of this:
#    %_generate.go: $(shell find $(dir $@)/assets -type f)
#

clean-ui-assets:
	$(Q)echo "Cleaning up generated UI assets..."; \
	for i in $(UI_ASSETS); do \
	    echo "  - $$i"; \
	    rm -f $$i; \
	done

%_vfsdata.go:: %_generate.go
	$(Q)echo "Generating $@..."; \
	cd $(dir $@) && \
	    $(GO_GEN) || exit 1 && \
	cd - > /dev/null

#
# dependencies for UI assets baked in using vfsgendev (can't come up with a working pattern rule)
#

pkg/cri/resource-manager/visualizer/bubbles/assets_vfsdata.go:: \
	$(wildcard pkg/cri/resource-manager/visualizer/bubbles/assets/*.html) \
	$(wildcard pkg/cri/resource-manager/visualizer/bubbles/assets/js/*.js) \
	$(wildcard pkg/cri/resource-manager/visualizer/bubbles/assets/css/*.css)


# phony targets
.PHONY: all build install clean test images \
	format vet cyclomatic-check lint golangci-lint \
	git-version git-buildid

# We use bashisms in this Makefile.
SHELL := /bin/bash

# Go compiler/toolchain and extra related binaries we ues/need.
GO_PARALLEL :=
GO_CMD      := go
GO_BUILD    := $(GO_CMD) build $(GO_PARALLEL)
GO_GEN      := $(GO_CMD) generate -x
GO_FMT      := gofmt
GO_CYCLO    := gocyclo
GO_LINT     := golint
GO_CILINT   := golangci-lint

# TEST_TAGS is the set of extra build tags passed for tests.
# We disable AVX collector for tests by default.
TEST_TAGS := noavx,test
GO_TEST   := $(GO_CMD) test $(GO_PARALLEL) -tags $(TEST_TAGS)
GO_VET    := $(GO_CMD) vet -tags $(TEST_TAGS)

# Disable some golangci_lint checkers for now until we have an more acceptable baseline...
GO_CILINT_CHECKERS := -D unused,staticcheck,errcheck,deadcode,structcheck,gosimple -E golint,gofmt
GO_CILINT_RUNFLAGS := --build-tags $(TEST_TAGS)

# Protoc compiler and protobuf definitions we might need to recompile.
PROTOC    := $(shell command -v protoc;)
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
CONFIGDIR  ?= /etc/cri-resmgr
INSTALL    := install

# Directories (in cmd) with go code we'll want to build and install.
BUILD_DIRS = $(shell find cmd -name \*.go | sed 's:cmd/::g;s:/.*::g' | uniq)
BUILD_BINS = $(foreach dir,$(BUILD_DIRS),bin/$(dir))

# Directories (in cmd) with go code we'll want to create Docker images from.
IMAGE_DIRS  = $(shell find cmd -name Dockerfile | sed 's:cmd/::g;s:/.*::g' | uniq)
IMAGE_VERSION  := $(shell git describe --dirty 2> /dev/null || echo unknown)
ifdef IMAGE_REPO
    override IMAGE_REPO := $(IMAGE_REPO)/
endif

# List of our active go modules.
GO_MODULES = $(shell $(GO_CMD) list ./... | grep -v vendor/)
GO_PKG_SRC = $(shell find pkg -name \*.go)

# List of visualizer collateral files to go generate.
UI_ASSETS := $(shell for i in pkg/cri/resource-manager/visualizer/*; do \
        if [ -d "$$i" -a -e "$$i/assets_generate.go" ]; then \
            echo $$i/assets_gendata.go; \
        fi; \
    done)

# Right now we don't depend on libexec/%.o on purpose so make sure the file
# is always up-to-date when elf/avx512.c is changed.
GEN_TARGETS := pkg/avx/programbytes_gendata.go

# Determine binary version and buildid, and versions for rpm, deb, and tar packages.
BUILD_VERSION := $(shell scripts/build/get-buildid --version --shell=no)
BUILD_BUILDID := $(shell scripts/build/get-buildid --buildid --shell=no)
RPM_VERSION   := $(shell scripts/build/get-buildid --rpm --shell=no)
DEB_VERSION   := $(shell scripts/build/get-buildid --deb --shell=no)
TAR_VERSION   := $(shell scripts/build/get-buildid --tar --shell=no)

# Git (tagged) version and revisions we'll use to linker-tag our binaries with.
RANDOM_ID := "$(shell head -c20 /dev/urandom | od -An -tx1 | tr -d ' \n')"
LDFLAGS    = \
    -ldflags "-X=github.com/intel/cri-resource-manager/pkg/version.Version=$(BUILD_VERSION) \
             -X=github.com/intel/cri-resource-manager/pkg/version.Build=$(BUILD_BUILDID) \
             -B 0x$(RANDOM_ID)"

# Build non-optimized version for debugging on make DEBUG=1.
DEBUG ?= 0
ifeq ($(DEBUG),1)
    GCFLAGS=-gcflags "all=-N -l"
else
    GCFLAGS=
endif

# tar-related commands and options.
TAR        := tar
TAR_UPDATE := $(TAR) -uf
GZIP       := gzip
GZEXT      := .gz

# Metadata for packages, changelog, etc.
USER_NAME  ?= $(shell git config user.name)
USER_EMAIL ?= $(shell git config user.email)
BUILD_DATE ?= $(shell date -R)

# RPM spec files we might want to generate.
SPEC_FILES = $(shell find packaging -name \*.spec.in | sed 's/.spec.in/.spec/g' | uniq)

# Systemd collateral.
SYSTEMD_DIRS = $(shell find cmd -name \*.service -o -name \*.socket | sed 's:cmd/::g;s:/.*::g'|uniq)
SYSCONF_DIRS = $(shell find cmd -name \*.sysconf | sed 's:cmd/::g;s:/.*::g' | uniq)

# Extra options to pass to docker (for instance --network host).
DOCKER_OPTIONS =

# Docker boilerplate/commands to build debian/ubuntu packages.
DOCKER_DEB_BUILD := \
    cd /build && \
    tar -xvf /build/input/cri-resource-manager-$(TAR_VERSION).tar.gz && \
    cd cri-resource-manager-$(TAR_VERSION) && \
    cp -r /build/input/debian . && \
    dpkg-buildpackage -uc && \
    cp ../*.{buildinfo,changes,deb,dsc} /output

# Docker boilerplate/commands to build rpm packages.
DOCKER_RPM_BUILD := \
    mkdir -p ~/rpmbuild/{SOURCES,SPECS} && \
    cp -v /build/input/*.spec ~/rpmbuild/SPECS && \
    cp -v /build/input/*.tar.* ~/rpmbuild/SOURCES && \
    for spec in ~/rpmbuild/SPECS/*.spec; do \
        rpmbuild -bb $$spec; \
    done && \
    cp -v $$(rpm --eval %{_rpmdir}/%{_arch})/*.rpm /output

# Directory to leave built distro packages and collateral in.
PACKAGES_DIR := packages

# Directory to use to build distro packages.
BUILD_DIR := build

# dist tarball target name
ifneq ($(wildcard .git/.),)
    DIST_TARGET = dist-git
else
    DIST_TARGET = dist-cwd
endif

# Paths to exclude from tarballs generated by dist-cwd.
DIST_EXCLUDE := \
    --exclude="./$$tarball*" \
    --exclude='./cri-resource-manager-*' \
    --exclude='./$(PACKAGES_DIR)*' \
    --exclude='./$(BUILD_DIR)*'

# Path name transformations for tarballs generated by dist-cwd.
DIST_TRANSFORM := \
    --transform='s:^.:cri-resource-manager-$(TAR_VERSION):'

# Determine distro ID, version and package type.
DISTRO_ID      := $(shell . /etc/os-release; echo "$${ID:-unknown}")
DISTRO_VERSION := $(shell . /etc/os-release; echo "$${VERSION_ID:-unknown}")
DISTRO_PACKAGE := $(shell echo $(DISTRO_ID) | tr -d ' \t' | \
    sed -E 's/.*((centos)|(fedora)|(suse)).*/rpm/;s/.*((ubuntu)|(debian)).*/deb/')

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
    $(foreach dir,$(BUILD_DIRS),install-sysconf-$(dir)) \
    $(foreach dir,$(BUILD_DIRS),install-config-$(dir))


clean: $(foreach dir,$(BUILD_DIRS),clean-$(dir)) clean-spec clean-deb clean-ui-assets

images: $(foreach dir,$(IMAGE_DIRS),image-$(dir))

images-push: $(foreach dir,$(IMAGE_DIRS),image-push-$(dir))

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
	echo "Building $@ (version $(BUILD_VERSION), build $(BUILD_BUILDID))..."; \
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
	for f in $$(find $$dir -name \*.service -o -name \*.socket); do \
	    echo "  $$f in $(DESTDIR)$(UNITDIR)..."; \
	    $(INSTALL) -m 0644 -t $(DESTDIR)$(UNITDIR) $$f; \
	done

install-sysconf-%:
	$(Q)bin=$(patsubst install-sysconf-%,%,$@); dir=cmd/$$bin; \
	echo "Installing sysconf collateral for $$bin..."; \
	$(INSTALL) -d $(DESTDIR)$(SYSCONFDIR)/sysconfig && \
	for f in $$(find $$dir -name \*.sysconf); do \
	    echo "  $$f in $(DESTDIR)$(SYSCONFDIR)/sysconfig..."; \
	    df=$${f##*/}; df=$${df%.sysconf}; \
	    $(INSTALL) -m 0644 -T $$f $(DESTDIR)$(SYSCONFDIR)/sysconfig/$$df; \
	done

install-config-%:
	$(Q)bin=$(patsubst install-config-%,%,$@); dir=cmd/$$bin; \
	echo "Installing sample configuration collateral for $$bin..."; \
	$(INSTALL) -d $(DESTDIR)$(CONFIGDIR) && \
	for f in $$(find $$dir -name \*.cfg.sample); do \
	    echo "  $$f in $(DESTDIR)$(CONFIGDIR)..."; \
	    df=$${f##*/}; \
	    $(INSTALL) -m 0644 -T $$f $(DESTDIR)$(CONFIGDIR)/$${df%.sample}; \
	done

clean-%:
	$(Q)bin=$(patsubst clean-%,%,$@); src=cmd/$$bin; \
	echo "Cleaning up $$bin..."; \
	rm -f bin/$$bin

clean-gen:
	$(Q)rm -f $(GEN_TARGETS)

image-%:
	$(Q)bin=$(patsubst image-%,%,$@); \
		docker build . -f "cmd/$$bin/Dockerfile" -t $(IMAGE_REPO)$$bin:$(IMAGE_VERSION)

image-push-%: image-%
	$(Q)bin=$(patsubst image-push-%,%,$@); \
		if [ -z "$(IMAGE_REPO)" ]; then echo "ERROR: no IMAGE_REPO specified"; exit 1; fi; \
		docker push $(IMAGE_REPO)$$bin:$(IMAGE_VERSION)

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
	$(Q)$(GO_VET) $(GO_MODULES)

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
	$(Q)$(GO_CILINT) run $(GO_CILINT_RUNFLAGS) $(GO_CILINT_CHECKERS)


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

race-test racetest:
ifndef WHAT
	$(Q)$(GO_TEST) -race -coverprofile=coverage.txt -covermode=atomic \
	    $(GO_MODULES)
else
	$(Q)cd $(WHAT) && \
	    $(GO_TEST) -race -coverprofile=cover.out -covermode=atomic || rc=1; \
            $(GO_CMD) tool cover -html=cover.out -o coverage.html; \
            rm cover.out; \
            echo "Coverage report: file://$$(realpath coverage.html)"; \
            exit $$rc
endif

#
# Rules for building distro packages.
#

ifneq ($(DISTRO_ID),fedora)
    packages: cross-$(DISTRO_PACKAGE).$(DISTRO_ID)-$(DISTRO_VERSION)
else
    packages: cross-$(DISTRO_PACKAGE).$(DISTRO_ID)
endif

#
# Rules for building dist-tarballs, rpm, and deb packages.
#

dist: $(DIST_TARGET)

dist-git:
	$(Q)echo "Using git to create dist tarball $(TAR_VERSION) from $(BUILD_BUILDID)..."; \
	tardir=cri-resource-manager-$(TAR_VERSION) && \
	tarball=cri-resource-manager-$(TAR_VERSION).tar && \
	git archive --format=tar --prefix=$$tardir/ HEAD > $$tarball && \
	mkdir -p $$tardir && \
	    echo $(BUILD_VERSION) > $$tardir/version && \
	    echo $(BUILD_BUILDID) > $$tardir/buildid && \
	$(TAR) -uf $$tarball $$tardir && \
	rm -f $$tarball.* && \
	$(GZIP) $$tarball && \
	rm -fr $$tardir

dist-cwd:
	$(Q)echo "Using tar to create dist tarball $(TAR_VERSION) from $$(pwd)..."; \
	tardir=cri-resource-manager-$(TAR_VERSION) && \
	tarball=cri-resource-manager-$(TAR_VERSION).tar && \
	$(TAR) $(DIST_EXCLUDE) $(DIST_TRANSFORM) -cvf - . > $$tarball && \
	mkdir -p $$tardir && \
	    echo $(BUILD_VERSION) > $$tardir/version && \
	    echo $(BUILD_BUILDID) > $$tardir/buildid && \
	$(TAR_UPDATE) $$tarball $$tardir && \
	rm -f $$tarball.* && \
	$(GZIP) $$tarball && \
	rm -fr $$tardir

spec: clean-spec $(SPEC_FILES)

%.spec:
	$(Q)echo "Generating RPM spec file $@..."; \
	cp $@.in $@ && \
	sed -E -i -e "s/__VERSION__/$(RPM_VERSION)/g"    \
	          -e "s/__TARVERSION__/$(TAR_VERSION)/g" \
	          -e "s/__BUILDID__/$(BUILD_BUILDID)/g" $@

clean-spec:
	$(Q)rm -f $(SPEC_FILES)

cross-rpm.%: docker/cross-build/% clean-spec spec dist
	$(Q)distro=$(patsubst cross-rpm.%,%,$@); \
	builddir=$(BUILD_DIR)/docker/$$distro; \
	outdir=$(PACKAGES_DIR)/$$distro; \
	echo "Docker cross-building $$distro packages..."; \
	mkdir -p $(PACKAGES_DIR)/$$distro && \
	rm -fr $$builddir && mkdir -p $$builddir/{input,build} && \
	cp cri-resource-manager-$(TAR_VERSION).tar$(GZEXT) $$builddir/input && \
	cp packaging/rpm/cri-resource-manager.spec $$builddir/input && \
	docker run --rm -ti $(DOCKER_OPTIONS) --user $(shell echo $$USER) \
	    --env USER_NAME="$(USER_NAME)" --env USER_EMAIL=$(USER_EMAIL) \
	    -v $$(pwd)/$$builddir:/build \
	    -v $$(pwd)/$$outdir:/output \
	    $$distro-build /bin/bash -c '$(DOCKER_RPM_BUILD)' && \
	rm -fr $$builddir

src.rpm source-rpm: spec dist
	mkdir -p ~/rpmbuild/{SOURCES,SPECS} && \
	cp packaging/rpm/cri-resource-manager.spec ~/rpmbuild/SPECS && \
	cp cri-resource-manager-$(TAR_VERSION).tar$(GZEXT) ~/rpmbuild/SOURCES && \
	rpmbuild -bs ~/rpmbuild/SPECS/cri-resource-manager.spec

rpm: source-rpm
	rpmbuild -bb ~/rpmbuild/SPECS/cri-resource-manager.spec

debian/%: packaging/deb.in/%
	$(Q)echo "Generating debian packaging file $@..."; \
	tardir=cri-resource-manager-$(TAR_VERSION) && \
	tarball=cri-resource-manager-$(TAR_VERSION).tar && \
	mkdir -p debian; \
	cp $< $@ && \
	sed -E -i -e "s/__PACKAGE__/cri-resource-manager/g" \
	          -e "s/__TARBALL__/$$tarball/g"            \
	          -e "s/__VERSION__/$(DEB_VERSION)/g"       \
	          -e "s/__AUTHOR__/$(USER_NAME)/g"          \
	          -e "s/__EMAIL__/$(USER_EMAIL)/g"          \
	          -e "s/__DATE__/$(BUILD_DATE)/g"           \
	          -e "s/__BUILD_DIRS__/$(BUILD_DIRS)/g" $@

clean-deb:
	$(Q)rm -fr debian

cross-deb.%: docker/cross-build/% \
    clean-deb debian/changelog debian/control debian/rules debian/compat dist
	$(Q)distro=$(patsubst cross-deb.%,%,$@); \
	echo "Docker cross-building $$distro packages..."; \
	builddir=$(BUILD_DIR)/docker/$$distro; \
	outdir=$(PACKAGES_DIR)/$$distro; \
	mkdir -p $(PACKAGES_DIR)/$$distro && \
	rm -fr $$builddir && mkdir -p $$builddir/{input,build} && \
	cp cri-resource-manager-$(TAR_VERSION).tar$(GZEXT) $$builddir/input && \
	cp -r debian $$builddir/input && \
	docker run --rm -ti $(DOCKER_OPTIONS) --user $(shell echo $$USER) \
	    --env USER_NAME="$(USER_NAME)" --env USER_EMAIL=$(USER_EMAIL) \
	    -v $$(pwd)/$$builddir:/build \
	    -v $$(pwd)/$$outdir:/output \
	    $$distro-build /bin/bash -c '$(DOCKER_DEB_BUILD)' && \
	rm -fr $$builddir

deb: debian/changelog debian/control debian/rules debian/compat dist
	dpkg-buildpackage -uc

# Build a docker image (for distro cross-building).
docker/cross-build/%: dockerfiles/cross-build/Dockerfile.%
	$(Q)distro=$(patsubst docker/cross-build/%,%,$@) && \
	echo "Building cross-build docker image for $$distro..." && \
	img=$${distro}-build && docker rm $$distro-build || : && \
	scripts/build/docker-build-image $$distro-build --container $(DOCKER_OPTIONS)

# Rule for recompiling a changed protobuf.
%.pb.go: %.proto
	$(Q)if [ -n "$(PROTOC)" -o ! -e "$@" ]; then \
	        echo "Generating go code ($@) for updated protobuf $<..."; \
	        $(PROTOC) -I . $< --go_out=plugins=grpc:.; \
	else \
	        echo "WARNING: no protoc found, compiling with OUTDATED $@..."; \
	fi


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

bin/cri-resmgr: $(wildcard cmd/cri-resmgr/*.go) $(UI_ASSETS) $(GEN_TARGETS) \
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

bin/webhook: $(wildcard cmd/cri-resmgr-webhook/*.go) \
    $(shell for dir in \
                  $(shell go list -f '{{ join .Deps  "\n"}}' ./cmd/cri-resmgr-webhook/... | \
                          grep cri-resource-manager/pkg/ | \
                          sed 's#github.com/intel/cri-resource-manager/##g'); do \
                find $$dir -name \*.go; \
            done | sort | uniq)

#
# rules to run go generators
#
clean-ui-assets:
	$(Q)echo "Cleaning up generated UI assets..."; \
	for i in $(UI_ASSETS); do \
	    echo "  - $$i"; \
	    rm -f $$i; \
	done

%_gendata.go::
	$(Q)echo "Generating $@..."; \
	cd $(dir $@) && \
	    $(GO_GEN) || exit 1 && \
	cd - > /dev/null

#
# dependencies for UI assets baked in using vfsgendev (can't come up with a working pattern rule)
#

pkg/cri/resource-manager/visualizer/bubbles/assets_gendata.go:: \
	$(wildcard pkg/cri/resource-manager/visualizer/bubbles/assets/*.html) \
	$(wildcard pkg/cri/resource-manager/visualizer/bubbles/assets/js/*.js) \
	$(wildcard pkg/cri/resource-manager/visualizer/bubbles/assets/css/*.css)


# phony targets
.PHONY: all build install clean test images images-push\
	format vet cyclomatic-check lint golangci-lint

SPHINXOPTS    =
SPHINXBUILD   = sphinx-build
SOURCEDIR     = .
BUILDDIR      = _build

# Generate doc site under _build/html with Sphinx.
vhtml: _work/venv/.stamp
	. _work/venv/bin/activate && \
		$(SPHINXBUILD) -M html "$(SOURCEDIR)" "$(BUILDDIR)" $(SPHINXOPTS) $(O) && \
		cp docs/html/index.html $(BUILDDIR)/html/index.html

html:
		$(SPHINXBUILD) -M html "$(SOURCEDIR)" "$(BUILDDIR)" $(SPHINXOPTS) $(O) && \
		cp docs/html/index.html $(BUILDDIR)/html/index.html

clean-html:
	rm -rf $(BUILDDIR)/html

# Set up a Python3 environment with the necessary tools for document creation.
_work/venv/.stamp: docs/requirements.txt
	rm -rf ${@D}
	python3 -m venv ${@D}
	. ${@D}/bin/activate && pip install -r $<
	touch $@

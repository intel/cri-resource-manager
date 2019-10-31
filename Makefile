# Go compiler/toolchain and extra related binaries we ues/need.
GO_CMD    := go
GO_BUILD  := $(GO_CMD) build
GO_FMT    := gofmt
GO_CYCLO  := gocyclo
GO_LINT   := golint
GO_CILINT := golangci-lint

# Disable some golangci_lint checkers for now until we have an more acceptable baseline...
GO_CILINT_CHECKERS := -D unused,staticcheck,errcheck,deadcode,structcheck,gosimple -E golint,gofmt

# Protoc compiler and protobuf definitions we might need to recompile.
PROTOC    := protoc
PROTOBUFS  = $(shell find cmd pkg -name \*.proto)
PROTOCODE := $(patsubst %.proto,%.pb.go,$(PROTOBUFS))

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

# Git (tagged) version and revisions we'll use to linker-tag our binaries with.
GIT_ID   = scripts/build/git-id
BUILD_ID = "$(shell head -c20 /dev/urandom | od -An -tx1 | tr -d ' \n')"
LDFLAGS  = -ldflags "-X=github.com/intel/cri-resource-manager/pkg/version.Version=$$gitversion \
                     -X=github.com/intel/cri-resource-manager/pkg/version.Build=$$gitbuildid \
                     -B 0x$(BUILD_ID)"

# Tarball timestamp, tar-related commands.
TAR          := tar
TIME_STAMP   := $(shell date +%Y%m%d)
TAR_UPDATE   := $(TAR) -uf
TAR_COMPRESS := bzip2
TAR_SUFFIX   := bz2

# RPM spec files we might want to generate.
SPEC_FILES = $(shell find packaging -name \*.spec.in | sed 's/.spec.in/.spec/g' | uniq)

# Systemd collateral.
SYSTEMD_DIRS = $(shell find cmd -name \*.service -o -name \*.socket | sed 's:cmd/::g;s:/.*::g'|uniq)
SYSCONF_DIRS = $(shell find cmd -name \*.sysconf | sed 's:cmd/::g;s:/.*::g' | uniq)

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

clean: $(foreach dir,$(BUILD_DIRS),clean-$(dir)) clean-spec

images: $(foreach dir,$(IMAGE_DIRS),image-$(dir))

#
# Rules for building and installing binaries, or building docker images, and cleaning up.
#

bin/%:
	$(Q)bin=$(notdir $@); src=cmd/$$bin; \
	eval `$(GIT_ID)`; \
	echo "Building $@ (version $$gitversion, build $$gitbuildid)..."; \
	mkdir -p bin && \
	cd $$src && \
	    $(GO_BUILD) $(LDFLAGS) -o ../../bin/$$bin

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
	        scripts/build/docker-build --network=host $$buildopts $$src; \
	        rc=$$?; \
	    rm -fr vendor; \
	    exit $$rc

#
# Rules for format checking, various code quality and complexity checks and measures.
#

format:
	$(Q)report=`$(GO_FMT) -s -d -w $$(find cmd pkg -name \*.go)`; \
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
	$(Q)$(GO_CMD) test -race -coverprofile=coverage.txt -covermode=atomic \
	    $(GO_MODULES)

#
# Rules for building dist-tarballs, SPEC-files and RPMs.
#

dist:
	$(Q)eval `$(GIT_ID) .` && \
	tarid=`echo $$gitversion | tr '+-' '_'` && \
	tardir=cri-resource-manager-$$tarid; \
	tarball=cri-resource-manager-$$tarid.tar; \
	echo "Creating $$tarball.$(TAR_SUFFIX)..."; \
	rm -fr $$tardir $$tarball* && \
	git archive --format=tar --prefix=$$tardir/ HEAD > $$tarball && \
	mkdir -p $$tardir && cp git-{version,buildid} $$tardir && \
	$(TAR_UPDATE) $$tarball $$tardir && \
	$(TAR_COMPRESS) $$tarball && \
	rm -fr $$tardir

spec: clean-spec $(SPEC_FILES)

%.spec:
	$(Q)echo "Generating RPM spec file $@..."; \
	eval `$(GIT_ID)`; \
	tarid=`echo $$gitversion | tr '+-' '_'`; \
	cat $@.in | sed "s/__VERSION__/$$tarid/g;s/__BUILDID__/$$gitbuildid/g" \
	    > $@

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

bin/cri-resmgr: $(wildcard cmd/cri-resmgr/*.go) \
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

# phony targets
.PHONY: all build install clean test images \
	format vet cyclomatic-check lint golangci-lint \
	git-version git-buildid

# Expected Go version 1.12.x
GO := go
# Gofmt formats Go programs
GOFMT := gofmt
# Calculate cyclomatic complexities of Go functions
# go get -u github.com/fzipp/gocyclo
GOCYCLO := gocyclo
# Golint is a linter for Go source code
# go get -u golang.org/x/lint/golint
GOLINT := golint
# Gofluff relaxes linting a bit, allowing one to whitelist locally preferred terms.
GOFLUFF := scripts/hacking/gofluff
# We insist on using Cpu and Id, despite of golint suggesting on CPU and ID.
# We also use in one special case context.Context as a non-first parameter.
# Silence these complaints of golint.
GOFLUFF_DEBUG :=
GOFLUFF_WHITELIST := \
  --whitelist Cpu=CPU,Id=ID,Uid=UID \
  --allow-nonfirst relayWithCacheUpdate=context.Context \
  --allow-nonfirst relayWithNoActions=context.Context \
  --allow-nonfirst filterWithFakeSuccess=context.Context \
  --allow-nonfirst interceptWithPolicy=context.Context \
  --allow-blankimport ./pkg/cri-resource-manager/builtin-policies.go

# Commands to build by the build target.
COMMANDS = $(shell ls cmd)

# Protobuf compiler and code we might need to update/generate.
PROTOC := protoc
PROTOBUFS := $(shell find pkg -name \*.proto)
PROTOCODE := $(patsubst %.proto,%.pb.go,$(PROTOBUFS))

# Directories to docker build in, override tag to use, registry to docker push to.
DOCKERDIRS = $(shell ls cmd/*/Dockerfile | sed 's:/Dockerfile::g')
DOCKERTAG = ""
DOCKERPUSH = ""
# Path to kubelet binary
KUBECTL=$(shell which kubectl)

all: build # images

# build the given commands
build: $(PROTOCODE) $(COMMANDS)

format:
	@report=`$(GOFMT) -s -d -w $$(find cmd pkg -name \*.go)` ; if [ -n "$$report" ]; then echo "$$report"; exit 1; fi

vet:
	@$(GO) vet $(shell $(GO) list ./... | grep -v vendor)

cyclomatic-check:
	@report=`$(GOCYCLO) -over 15 cmd pkg`; if [ -n "$$report" ]; then echo "Complexity is over 15 in"; echo $$report; exit 1; fi

lint:
	@rc=0 ; for f in $$(find -name \*.go | grep -v \.\/vendor) ; do $(GOLINT) -set_exit_status $$f || rc=1 ; done ; exit $$rc

fluff:
	@rc=0 ; for f in $$(find -name \*.go | grep -v \.\/vendor) ; do $(GOFLUFF) $(GOFLUFF_DEBUG) $(GOFLUFF_WHITELIST) -set_exit_status $$f || rc=1 ; done ; exit $$rc

# clean the given commands
clean:
	@for cmd in $(COMMANDS); do \
	    echo "Cleaning up $$cmd..." && \
	    cd cmd/$$cmd && \
	    go clean && \
	    cd - > /dev/null 2>&1; \
	done

# build the given images
images: $(DOCKERDIRS)
	@for dd in $(DOCKERDIRS); do \
	    echo "Building docker image for $$dd"; \
	    buildopts="--quiet"; \
	    if [ -n "$(DOCKERTAG)" ]; then \
		buildopts="$$buildopts --tag $(DOCKERTAG)"; \
	    fi; \
	    if [ -n "$(DOCKERPUSH)" ]; then \
	        buildopts="$$buildopts --registry $(DOCKERPUSH)"; \
	    fi; \
	    scripts/build/docker-build $$buildopts $$dd; \
	done

# try to kube-reconfigure and redeploy quasi-random stuff we find lying around
deploy: images
	@if [ -n "$(KUBECTL)" ]; then \
	    for dd in $(DOCKERDIRS); do \
	        echo "Kube-redeploying $$dd..."; \
	        for yml in $dd/*config.yaml; do \
		    echo "Updating configuration $yml..."; \
		    $(KUBECTL) apply -f $$yml; \
		done; \
	        for yml in $dd/*deployment.yaml; do \
		    echo "Updating deployment $yml..."; \
		    $(KUBECTL) apply -f $$yml; \
		done; \
	    done; \
	fi

# go-build the given commands
$(COMMANDS): check-git-hooks
	@echo "Building $@..." && \
	    cd cmd/$@ && \
	    GO111MODULE=on go build -o $@

%.pb.go: %.proto
	@echo "Generating $@..."
	@$(PROTOC) -I . $< --go_out=plugins=grpc:.

# check and redirect git hooks to our in-repo hook directory
check-git-hooks:
	@if [ ! -e .git-hooks.redirected ]; then \
	    echo -n "Redirecting git hooks to .githooks..."; \
	    git config core.hookspath .githooks && \
	      touch .git-hooks.redirected && \
	    echo "done."; \
	fi

.PHONY: all build format vet cyclomatic-check lint clean images $(COMMANDS) $(IMAGES)

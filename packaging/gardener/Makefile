EXTENSION_IMAGE_NAME        := gardener-extension-cri-rm
INSTALLATION_IMAGE_NAME     := gardener-extension-cri-rm-installation
VERSION 					:= latest
CRI_RM_VERSION 				:= 0.6.0
ARCHIVE_NAME 				:= cri-resource-manager-$(CRI_RM_VERSION).x86_64.tar.gz
CRI_RM_URL 					:= https://github.com/intel/cri-resource-manager/releases/download/v$(CRI_RM_VERSION)/$(ARCHIVE_NAME)

.PHONY: start
start:
	go run ./cmd/gardener-extension-cri-rm --ignore-operation-annotation=false --leader-election=false

.PHONY: install
install:
	# TODO: Flags/version
	go install ./...

.PHONY: install-binaries
install-binaries:
	wget $(CRI_RM_URL)
	# TODO: requires GLIBC_2.32 -- needs to match target distro need to consider building cri-rm from scratch
	tar -xzvf $(ARCHIVE_NAME) -C / 

.PHONY: docker-images
docker-images:
	docker build -t $(EXTENSION_IMAGE_NAME):$(VERSION) -f Dockerfile --target $(EXTENSION_IMAGE_NAME) .
	docker build -t $(INSTALLATION_IMAGE_NAME):$(VERSION) -f Dockerfile --target $(INSTALLATION_IMAGE_NAME) .



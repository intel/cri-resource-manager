all: up

install:
	vagrant plugin install dotenv
	vagrant plugin install vagrant-proxyconf
	vagrant plugin install vagrant-libvirt

up: prepare
	vagrant up

destroy: up
	vagrant destroy --force --graceful --no-tty

validate: up
	vagrant validate

BASE_PACKAGE=`pwd`/../../output/provisioned-ubuntu2004.box

prepare:
	./create-vagrantfile.sh $(BASE_PACKAGE)

cleanup:
	rm -r .vagrant
	rm -f Vagrantfile

#!/usr/bin/env bash

# This will create the VM from vagrant box file, start the
# runner and wait for connections. After the run, the VM is
# destroyed and then re-created.

STOPPED=0
trap ctrl_c INT TERM

ctrl_c() {
    STOPPED=1
}

wait_for_ssh() {
    while [ "$(vagrant status --machine-readable 2>/dev/null | awk -F, '/,state,/ { print $4 }')" != "running" ]; do
        echo "Waiting for VM SSH server to respond"
        sleep 1

	if [ $STOPPED -eq 1 ]; then
	    echo "Exiting."
	    exit 1
	fi
    done
}

export BASE_PACKAGE=`pwd`/../../output/provisioned-ubuntu2004.box

if [ ! -f $BASE_PACKAGE ]; then
    echo "Create a box file $BASE_PACKAGE that contains the provisioned environment."
    echo "Follow the instructions in ../../README.md file and provision the base image first."
    exit 1
fi

while [ $STOPPED -eq 0 ]; do
  # All the configuration data, like runner URL and token, is stored in env file.
  . ../../env

  make up

  wait_for_ssh

  # Generate a key that is required by e2e tests. Also start agent to serve keys.
  vagrant ssh -c "rm -f ~/.ssh/id_rsa* > /dev/null 2>&1 ; ssh-keygen -q -V -1m:forever -P '' -t rsa  -f ~/.ssh/id_rsa"
  vagrant ssh -c 'eval $(ssh-agent); ssh-add'

  # See https://docs.github.com/en/actions/hosting-your-own-runners/autoscaling-with-self-hosted-runners for
  # info about the choosen options.
  vagrant ssh -c "cd actions-runner; ./config.sh --replace --name '$GHA_RUNNER_NAME' --url '$GHA_RUNNER_URL' --token '$GHA_RUNNER_TOKEN' --ephemeral --unattended" | tee /dev/tty | egrep -q -e "Http response code: NotFound from " -e "Invalid configuration provided for url"
  if [ $? -eq 0 ]; then
      echo "Our action config disappeared from github, you need to re-create it and update env file with new token."
      STOPPED=1
  else
      vagrant ssh -c "cd actions-runner; ./run.sh" | tee /dev/tty | egrep -q -e "Exiting\.\.\." -e "An error occurred: Not configured\. Run config"
      if [ $? -eq 0 ]; then
	  # User stopped the script, we should quit
	  STOPPED=1
      fi
  fi

  make destroy
done

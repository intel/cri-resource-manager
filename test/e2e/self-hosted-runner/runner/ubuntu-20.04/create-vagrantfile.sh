#!/usr/bin/env bash

if [ -s Vagrantfile ]; then
    echo "Vagrantfile already exists, not overwriting it"
    exit 0
fi

ESCAPED_REPLACE=$(printf '%s\n' "$1" | sed -e 's/[\/&]/\\&/g')
sed "s/PROVISIONED-BOX-FILE/$ESCAPED_REPLACE/" Vagrantfile.template > Vagrantfile

#!/bin/bash

# This is a helper for running the identically named code-generator script from
# https://github.com/kubernetes/code-generator.

REPO=https://github.com/kubernetes/code-generator
SCRIPT="$(realpath "$0")"
HEADER="${SCRIPT%/*}"/boilerplate.go.txt
TOPDIR=${SCRIPT%/scripts/*}

MODDIR=$TOPDIR
MODURL=$(grep ^module "$TOPDIR"/go.mod | sed 's/^module *//g')
MODULES=${MODULES:-pkg/topology}

fail() {
    echo "error: $*"
    exit 1
}

# Parse $* for --output-base, set $gendir and $repo accordingly.
pick-gen-dir() {
    local _save="" _a

    gendir=$TOPDIR/generate
    for _a in "$@"; do
        case $_a in
            --output-base)
                _save=y;;
            *)
                if [ -n "$_save" ]; then
                    gendir=$_a
                    _save=""
                fi
                ;;
        esac
    done
    repo=$gendir/${REPO##*/}
}

# Set $tag to correspond to $KUBERNETE_VERSION.
pick-git-tag() {
    if [ -z "$KUBERNETES_VERSION" ]; then
        fail "KUBERNETES_VERSION not set, please set it to the desired version to match/use."
    fi
    case $KUBERNETES_VERSION in
        v1.[0-9.]*) tag=${KUBERNETES_VERSION/#v1/v0};;
        *)
            fail "Don't know how to convert KUBERNETES_VERSION $KUBERNETES_VERSION to tag."
            ;;
    esac
}

# Clone $REPO as $repo.
git-clone() {
    if [ ! -d "$repo"/.git ]; then
        mkdir -p "$gendir" || fail "failed to clone git repo"
        (cd "$gendir" && git clone $REPO) || fail "failed to clone git repo $REPO"
    else
        (cd "$repo" && git fetch -q origin) || fail "failed to update/fetch git repo $REPO"
    fi
}

# Check out the $tag corresponding to $KUBERNETES_VERSION.
git-switch() {
    (set -e
     cd "$repo"
         git reset -q --hard HEAD 2> /dev/null
         git checkout -q "$tag"
    ) || fail "failed to checkout git tag $tag"
}

# Patch $repo/go.mod with replacement rules from $TOPDIR and add replacement rules for $TOPDIR.
go-mod-patch() {
    (set -e
     cd "$repo"
         grep -A 640 '^replace ' "$TOPDIR"/go.mod | grep -v pkg/topology >> go.mod
         go mod edit -replace="$MODURL=$MODDIR"
         for mod in $MODULES; do
             go mod edit -replace="$MODURL/$mod=$MODDIR/$mod"
         done
    ) || fail "failed to patch go.mod"
}

# Check any previously generated files for $MODURL, bail out if they exist.
check-existing() {
    local _pkg=${3%:*} _ver=${3#*:} _dir
    for _dir in "$gendir/$1" "$gendir/$2/$_pkg/$_ver"; do
        if [ -d "$_dir" ]; then
            fail "$_dir already exists, refusing to overwrite it"
        fi
    done
}

# Run generate
run-generator() {
    (set -e
     cd "$repo"
         ./generate-groups.sh "$@" --go-header-file "$HEADER"
    ) || fail "code generation failed"
}

pick-gen-dir "$@"
pick-git-tag
git-clone
git-switch
go-mod-patch
check-existing "$2" "$3" "$4"
run-generator "$@"

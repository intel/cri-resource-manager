#!/bin/bash -e
set -o pipefail

script=`basename $0`

usage () {
cat << EOF
Usage: $script [-h] [-a] [BUILD_SUBDIR]

Options:
  -h         show this help and exit
  -a         amend (with --reset-author) instead of creating a new commit
EOF
}

# Helper function for detecting available versions from the current directory
create_versions_js() {
    _baseurl="/cri-resource-manager"

    echo -e "function getVersionsMenuItems() {\n  return ["
    # 'stable' is a symlink pointing to the latest version
    [ -f stable ] && echo "    { name: 'stable', url: '$_baseurl/stable' },"
    for f in `ls -d */  | tr -d /`; do
        echo "    { name: '$f', url: '$_baseurl/$f' },"
    done
    echo -e "  ];\n}"
}

#
# Argument parsing
#
while [ "${1#-}" != "$1" -a -n "$1" ]; do
    case "$1" in
        -a|--amend)
            amend="--amend --reset-author"
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            usage
            exit 1
            ;;
    esac
    shift
done

build_subdir="$1"

# Check that no extra args were provided
if [ $# -gt 1 ]; then
    echo "ERROR: unknown arguments: $@"
    usage
    exit 1
fi

#
# Build the documentation
#
build_dir="_build"
echo "Creating new Git worktree at $build_dir"
git worktree add "$build_dir" gh-pages

# Drop worktree on exit
trap "echo 'Removing Git worktree $build_dir'; git worktree remove --force '$build_dir'" EXIT

# Parse subdir name from GITHUB_REF
if [ -z "$build_subdir" ]; then
    case "$GITHUB_REF" in
        refs/tags/*)
            _base_ref=${GITHUB_REF#refs/tags/}
            ;;
        refs/heads/*)
            _base_ref=${GITHUB_REF#refs/heads/}
            ;;
        *) _base_ref=
    esac
    echo "Parsed baseref: '$_base_ref'"

    case "$GITHUB_REF" in
        refs/tags/v*)
            _version=${GITHUB_REF#refs/tags/v}
            ;;
        refs/heads/release-*)
            _version=${GITHUB_REF#refs/heads/release-}
            ;;
        *) _version=
    esac
    echo "Detected version: '$_version'"

    _version=`echo -n $_version | sed -nE s'!^([0-9]+\.[0-9]+).*$!\1!p'`

    # Use version as the subdir
    build_subdir=${_version:+v$_version}
    # Fallback to base-ref i.e. name of the branch or tag
    if [ -z "$build_subdir" ]; then
        # For master branch we use the name 'devel'
        [ "$_base_ref" = "master" ] && build_subdir=devel || build_subdir=$_base_ref
    fi
fi

# Default to 'devel' if no subdir was given and we couldn't parse
# it
build_subdir=${build_subdir:-devel}
echo "Updating site version subdir: '$build_subdir'"
export SITE_BUILDDIR="$build_dir/$build_subdir"
export VERSIONS_MENU=1

make html

#
# Update gh-pages branch
#
commit_hash=`git describe --dirty --always`

# Switch to work in the gh-pages worktree
cd "$build_dir"

# Add "const" files we need in root dir
touch .nojekyll

_stable=`ls -d1 v*/ || : | sort -n | tail -n1`
[ -n "$_stable" ] && ln -sfT "$_stable" stable

# Detect existing versions from the gh-pages branch
create_versions_js > versions.js

cat > index.html << EOF
<meta http-equiv="refresh" content="0; URL='stable'" />
EOF

if [ -z "`git status --short`" ]; then
    echo "No new content, gh-pages branch already up-to-date"
    exit 0
fi

# Create a new commit
commit_msg=`echo -e "Update documentation for $build_subdir\n\nAuto-generated from $commit_hash by '$script'"`

echo "Committing changes..."
# Exclude doctrees dir
git add -- ":!$build_subdir/.doctrees"
git commit $amend -m "$commit_msg"

cd -

echo "gh-pages branch successfully updated"

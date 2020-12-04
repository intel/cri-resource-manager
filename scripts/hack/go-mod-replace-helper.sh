#!/bin/bash -e
set -o pipefail

this=`basename $0`

usage () {
cat << EOF
USAGE: $this REPO_CACHE_DIR VERSION MODULE...

OPTIONS
  -h         show this help and exit

EXAMPLES
  Print replace directives for all k8s.io/* updated to v0.19.4:

    $ sed -n '/replace/,$p' go.mod  | grep k8s.io | awk '{print $1}' | \\
      xargs ./scripts/hack/go-mod-replace-helper.sh ../k8s-cache/ v0.19.4

EOF
}

update_cache() {
    local module_base=`basename "$1"`
    local module_cache_dir="$cache_dir/$module_base"


    if [ ! -e "$module_cache_dir" ]; then
        module_repo="https://github.com/kubernetes/$module_base"
        echo "Cloning $module_repo to $module_cache_dir"
        git clone -q --depth=1 "$module_repo" "$module_cache_dir"
    fi

    echo "Updating $1 at $module_cache_dir"
    cd "$module_cache_dir"
    git fetch -q --tags --depth=1
    cd ->/dev/null
}

gomodrev() {
    local module_base=`basename "$1"`
    local module_cache_dir="$cache_dir/$module_base"
    cd "$module_cache_dir"

    # Resolve to a commit
    sha=`git rev-parse "$2"~0`

    short_sha=`git rev-parse --short=12 $sha`

    unix_ts=`git show $sha --format=%ct --date=unix | head -n1`

    gomod_ts=`date -u --date=@$unix_ts +'%Y%m%d%H%M%S'`

    echo "v0.0.0-$gomod_ts-$short_sha"

    cd - >/dev/null
}

while [ "${1#-}" != "$1" -a -n "$1" ]; do
    case "$1" in
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

if [ $# -lt 3 ]; then
    usage
    exit 1
fi

cache_dir="$1"
shift
module_version="$1"
shift
module_names="$@"

cat << EOF

UPDATING CACHE
==============
EOF
for m in $@; do
    update_cache $m
done

cat << EOF

GO.MOD REPLACE
==============
EOF

for m in $@; do
    r=`gomodrev $m $module_version`
    echo -e "\t$m v0.0.0 => $m $r"
done

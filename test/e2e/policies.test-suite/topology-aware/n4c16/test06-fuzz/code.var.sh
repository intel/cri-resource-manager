source $TEST_DIR/codelib.sh || {
    echo "error importing codelib.sh"
    exit 1
}

# Clean test pods from the kube-system namespace
( kubectl delete pods -n kube-system $(kubectl get pods -n kube-system | awk '/t[0-9]r[gb][ue]/{print $1}') ) || true

# Run generated*.sh test scripts in this directory.
genscriptcount=0
for genscript in "$TEST_DIR"/generated*.sh; do
    if [ ! -f "$genscript" ]; then
        continue
    fi
    (
        paralleloutdir="$outdir/parallel$genscriptcount"
        [ -d "$paralleloutdir" ] && rm -rf "$paralleloutdir"
        mkdir "$paralleloutdir"
        OUTPUT_DIR="$paralleloutdir"
        COMMAND_OUTPUT_DIR="$paralleloutdir/commands"
        mkdir "$COMMAND_OUTPUT_DIR"
        source "$genscript" 2>&1 | sed -u -e "s/^/$(basename "$genscript"): /g"
    ) &
    genscriptcount=$(( genscriptcount + 1))
done

if [[ "$genscriptcount" == "0" ]]; then
    echo "WARNING:"
    echo "WARNING: Skipping fuzz tests:"
    echo "WARNING: - Generated tests not found."
    echo "WARNING: - Generate a test by running:"
    echo "WARNING:   $TEST_DIR/generate.sh"
    echo "WARNING: - See test generation options:"
    echo "WARNING:   $TEST_DIR/generate.sh --help"
    echo "WARNING:"
    sleep 5
    exit 0
fi

echo "waiting for $genscriptcount generated tests to finish..."
wait

# Restore default test configuration, restart cri-resmgr.
terminate cri-resmgr
cri_resmgr_cfg=$(instantiate cri-resmgr.cfg)
launch cri-resmgr

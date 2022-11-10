#!/usr/bin/env python3

# This is an example of an external heat forecaster process.
# This code can be used in conjuction with the "stdio" forecaster
# of the heat policy.
#
# policy:
#   name: heat
#   config: |
#     ...
#     forecaster:
#       name: stdio
#       config: |
#         command: ['python3', 'memtierd-stdio-forecaster.py']
#     ...
#
# Look for a full example under sample-configs/.

import sys
import json
import traceback

error_log_filename = "/tmp/forecaster.log"
debug_log_filename = "/tmp/forecaster.log"

# debug: write a debug message into a log file.
def debug(msg):
    open(debug_log_filename, "a").write("DEBUG forecaster.py: %s\n" % (msg,))

# error: write an error message into a log file, exit if status is given.
def error(msg, exit_status=None):
    open(error_log_filename, "a").write("ERROR forecaster.py: %s\n" % (msg,))
    if exit_status is not None:
        sys.exit(exit_status)

# receive_heats: return current heats received from memtierd.
def receive_heats():
    l = sys.stdin.readline()
    if len(l) == 0:
        return None
    debug("receive_heats: got %d characters" % (len(l),))
    return json.loads(l)

# send_forecast: send new heats forecast to memtierd.
def send_forecast(new_heats):
    json.dump(new_heats, sys.stdout, separators=(',',':'))
    sys.stdout.write("\n")
    sys.stdout.flush()

# make_forecast: use current and previous heats (and whatever else is
# appropriate) to come up with new heats. Returned heats will be the
# forecast that will be sent to memtierd.
def make_forecast(current_heats):
    # Implement forecasting logic in this function.

    # This sample implementation modifies the heat in the first memory
    # address range of the first pid to 0.424242.
    if len(current_heats) > 0:
        pid = sorted(current_heats.keys())[0]
        current_heats[pid][0]["heat"] = 0.424242

    # Return modified heat structure.
    return current_heats

# main: read current heats and send forecasts to memtierd.
def main():
    while True:
        debug("wait for current heats")
        current_heats = receive_heats()
        if current_heats is None:
            debug("end of input")
            break

        new_heats = make_forecast(current_heats)

        debug("send new heats")
        send_forecast(new_heats)
    debug("exiting")

# dump all exceptions to log files for debugging.
if __name__ == "__main__":
    try:
        main()
    except Exception as e:
        debug("error: %s" % (e,))
        debug("dump traceback to error log")
        error(traceback.format_exc(), exit_status=1)

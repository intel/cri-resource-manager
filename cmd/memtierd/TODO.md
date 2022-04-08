Features
--------

- memtierd prometheus export.

- Refactor cri-rm memorytiering to use pkg/memtier.

- Huge pages. Autodetect!

- CSV output from stats/dumps?

- memtierd: check free memory availability from the system before
  moving pages to a NUMA node.

- damon: drop pids that we are not monitoring, warn if someone else
  in the system seems to be using damon at the same time.

- running cost analysis utility: calculate CPU and memory consumption
  of running memtierd with different parameters.

- policy-tracker-interface: enable policy to receive event when new
  tracker values are available. Yet it may be sometimes useful to
  gather and aggregate tracker data from several rounds, some policies
  can be simpler to configure if they act only when new tracker data
  has become available. In practice this interval would be the same as
  tracker scan interval + scan time (idlepage, softdirty) or
  aggregation interval + time until the last entry from the dump is
  received (damon), yet the last one item can be difficult to
  identify.

- heatmap classification: increase configurability in heat
  classification:
  - absolute class sizes. Example:
    classSize: {4: "12 GB", 3: "8 GB", ...}
  - relative class sizes. for instance, 80 % of free memory on NUMA
    nodes that are (Mems_)allowed for the class. Example:
    classSize: {4: "80 %", 2: "10 %", ...}

  Alternative interpretations for class sizes:

  1. classSize limits the number of pages in a class. In this case
     classes can be filled up in different orders:

     1.1 Put a page to the hightest class that has room for
         it. Implementation example: sort by heat. Starting from the
         hottest page, put pages to classes starting from the highest
         class as long as there is room. This keeps all pages in best
         possible memory, even if they were cold. (Every class
         possibly except for the lowest class should have defined
         size, otherwise a class with undefined size is either left
         empty or turns into a black hole that sucks all remaining
         pages.) Best performance with the cost of other workloads.

     1.2 Put a page to its own class or lower if there is no
         room. Implementation example: sort by heat. Starting from the
         hottest page, calculate class corresponding to the heat of a
         page, and put the page to that class or the first class below
         it that has room. (Undefined class size is not a problem in
         this case.) Limited performance.

     Real implementations would only set heat watermarks for every
     class to be efficient.

  2. classSize limits the size of memory used on allowed target nodes
     of the class. If the memory capacity for the class is already
     used on target nodes, do not move pages to that node until free
     capacity becomes available. (If this kind of knob would be
     needed, maybe it should be named numaSize rather than class
     size.)

- tracker_damon: merging heatmap regions is needed to prevent data
  structure growth. Maybe merging should be done together with "cool
  down / clean up" update on regions that have not been seen for a
  long time.

- policy_age: support parameter tuning for optimal workload-specific configuration,
  maybe histogram/proportions of active/idle pages on varying durations.

- policy_heat: support parameter tuning for optimal workload-specific configuration,
  maybe histogram/proportions of pages of different heats on varying Max/Retention.

- policy_* memory nodes: Mask target nodes with /proc/pid/status:Mems_allowed

Optimizations
-------------

- policy_* memory nodes: If target node for page movement can be
  chosen, choose the best based on where most of the memory is already
  located. Take free memory on the node into account, too.

- Can we cache PFNs of page addresses? This would skip reading pagemap
  to find PFNs for reading kpageflags or page_idle/bitmap.

- An intelligent way to read flags/bits of transparent huge pages
  would avoid reading compound tail pages.

Done
----

- trackers: Re-read process address ranges.

- meme: Add options to dynamically change the size of exercised
  address ranges.

- trackers: Support overlapping address ranges in the heat
  conversion. Make sure this works with the counters from
  damon/idlepage/softdirty.

- policy_heat: Implement this policy to moves pages based on address
  range heat.

- policy_* memory nodes: Allow several target nodes per heat/age.

Features
--------

- running cost analysis utility: calculate CPU and memory consumption
  of running memtierd with different parameters.

- trackers: Re-read process address ranges.

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

- meme: Add options to dynamically change the size of exercised
  address ranges.

- trackers: Support overlapping address ranges in the heat
  conversion. Make sure this works with the counters from
  damon/idlepage/softdirty.

- policy_heat: Implement this policy to moves pages based on address
  range heat.

- policy_* memory nodes: Allow several target nodes per heat/age.

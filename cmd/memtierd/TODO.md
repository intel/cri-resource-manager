Features
--------

- meme: Add options to dynamically change the size of exercised
  address ranges.

- trackers: Support overlapping address ranges in the heat
  conversion. Make sure this works with the counters from
  damon/idlepage/softdirty.

- trackers: Re-read process address ranges.

- policy_heat: Implement this policy to moves pages based on address
  range heat.

- policy_* memory nodes: Allow several target nodes per heat/age.

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

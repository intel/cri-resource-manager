# Structure

```
                  Mover        (mover.go)
          handles MoverTasks   (mover.go)

... each active task has unmoved pages of a single

                  Process      (process.go)
         that has AddrRanges   (addrrange.go)
each of which has Pages        (page.go)

-----------------------+---------------------
proc.go                |  move_linux.go
-----------------------+---------------------
/proc/pid/maps         | syscall: move_pages
/proc/pid/numa_maps    |
/proc/pid/pagemap      |
```

AddrRanges enable finding suitable pages by
1. a subrange of addresses
2. rough number of pages per node
3. address range attributes (dirty, heap)

Pages enable finding suitable pages by pagetable page attributes.

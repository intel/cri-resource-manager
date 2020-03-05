// CRI-RM introspection data to UI JSON data format adaptation.

function AdaptJSON(data) {
    var root, nodes, containers

    console.log("should translate introspection to d3obj: %o", data)

    root = null
    nodes = new Object()
    containers = new Object()

    // create tree of pools
    for (var name in data.Pools) {
        p = data.Pools[name]

        console.log("got pool %o: %o", name, p)

        node = new Object()
        node.name     = p.Name
        node.CPUs     = p.CPUs
        node.Memory   = p.Memory
        node.children = new Array()
        if (p.Parent == "") {
            root = node
            console.log("root set to %o: %o", p.parent, node)
        }
        nodes[name] = node
    }
    for (var name in data.Pools) {
        p = data.Pools[name]
        n = nodes[name]
        if (n == null) {
            console.log("failed to look up node %o", name)
        }
        if (p.Children != null) {
            for (i = 0; i < p.Children.length; i++) {
                cname = p.Children[i]
                n.children.push(nodes[cname])
            }
        }
    }

    // create lookup table of containers
    for (var pid in data.Pods) {
        console.log("got pod %o", pid)

        p = data.Pods[pid]
        for (var cid in p.Containers) {
            console.log("got container %o", cid)

            c = p.Containers[cid]

            node = new Object()
            node.name = p.Name + ":" + c.Name
            node.CPURequest = c.CPURequest
            node.CPULimit = c.CPULimit
            node.MemoryRequest = c.MemoryRequest
            node.MemoryLimit = c.MemoryLimit
            node.Hints = c.Hints
            node.container = c
            containers[cid] = node
        }
    }

    // attach containers to pools
    for (var cid in data.Assignments) {
        console.log("got assignment for container %o", cid)

        a = data.Assignments[cid]
        n = containers[cid]

        shared = ""
        exclusive = ""
        if (a.SharedCPUs != "") {
            shared = "shared:"+a.SharedCPUs+"(share:"+a.CPUShare+")"
        }
        if (a.ExclusiveCPUs != "") {
            exclusive = "exclusive:"+a.ExclusiveCPUs
        }
        cpu = ""
        sep = ""
        if (exclusive != "") {
            cpus = exclusive
            sep = " + "
        }
        if (shared != "") {
            cpu += sep + shared
        }
        n.CPUs         = cpu
        n.Memory       = a.Memory
        n.RDTClass     = a.RDTClass
        n.BlockIOClass = a.BlockIOClass

        p = nodes[a.Pool]
        p.children.push(n)
    }

    console.log("translated object: %o", root)

    return root
}




var svg = d3.select("svg")
   .attr("preserveAspectRatio", "xMinYMin meet")
   .attr("viewBox", "0 0 800 800"),
    margin = 20,
    diameter = +svg.attr("width"),
    g = svg.append("g").attr("transform", "translate(" + diameter / 2 + "," + diameter / 2 + ")");

var green = d3.color("green");

var color = d3.scaleLinear()
    .domain([-1, 5])
    .range(["hsl(152,80%,80%)", "hsl(228,30%,40%)"])
    .interpolate(d3.interpolateHcl);

var pack = d3.pack()
    .size([diameter - margin, diameter - margin])
    .padding(100);

drawBubbleGraph("/introspect")

function drawBubbleGraph(filename) {
    console.log("redraw")

    g.selectAll("*").remove()

    d3.json(filename, function(error, introspectJSON) {
        if (error) throw error;

        root = AdaptJSON(introspectJSON)
        root = d3.hierarchy(root)
            .sum(function(d) { return d.CPURequest; })
            .sort(function(a, b) { console.log (b.value + " - " + a.value);return b.value - a.value; });


        var focus = root,
        nodes = pack(root).descendants(),
        view;
        console.log(nodes);
        var circle = g.selectAll("circle")
            .data(nodes)
            .enter().append("circle")
                .attr("class", function(d) { console.log("dx: " + d.x + " dy: " + d.y + " dr: " + d.r); console.log(d.data.name); d.parent ? d.children ? console.log("node") : console.log("node leaf") : console.log ("node root");  return d.parent ? d.children ? "node" : "node node--leaf" : "node node--root"; })
                .on("click", function(d) { if (focus !== d) zoom(d), d3.event.stopPropagation(); })
                .on("mouseover", function(d) {return d.children ? null : showData(d);})
                .on("mouseout", function(d) {return d.children ? null : clearData(d);})
                .style("fill", function(d) { return d.children ? color(d.depth) : null; })

        let innercircle = g.selectAll("innercircle")
          .data(nodes)
          .enter().append("circle")
          .attr("class", function(d) { return d.parent ? d.children ? "inner--node" : "inner--leaf" : "inner--root"; })

          let innerleaf = g.selectAll(".inner--leaf")
              .attr("r", function(d) {if (d.data.CPULimit || d.data.CPURequest) return (d.r * d.data.CPULimit / d.data.CPURequest);})
              .style("fill-opacity", 0.2)
              .on("click", function(d) { if (focus !== d) zoom(d), d3.event.stopPropagation(); })
              .style("fill", "red");

        var text = g.selectAll("text")
            .data(nodes)
            .enter().append("text")
                .attr("class", "label")
                .style("fill-opacity", function(d) { return d.parent === root ? 1 : 0; })
                .style("display", function(d) { return d.parent === root ? "inline" : "none"; })
                .text(function(d) { return d.data.name;});

        var node = g.selectAll("circle,innerleaf,text");

    svg
      .style("background", color(-1))
      .on("click", function() { zoom(root); });

    zoomTo([root.x, root.y, root.r * 2 + margin]);

    function zoom(d) {
        var focus0 = focus; focus = d;

        var transition = d3.transition()
            .duration(d3.event.altKey ? 7500 : 750)
            .tween("zoom", function(d) {
              var i = d3.interpolateZoom(view, [focus.x, focus.y, focus.r * 2 + margin]);
              return function(t) { zoomTo(i(t)); };
            });

        svg.transition().selectAll("text")
        .filter(function(d) { return d.parent === focus || this.style.display === "inline"; })
            .style("fill-opacity", function(d) { return d.parent === focus ? 1 : 0; })
            .on("start", function(d) { if (d.parent === focus) this.style.display = "inline"; })
            .on("end", function(d) { if (d.parent !== focus) this.style.display = "none"; });
    }
  function zoomTo(v) {
    var k = diameter / v[2]; view = v;
    node.attr("transform", function(d) { return "translate(" + (d.x - v[0]) * k + "," + (d.y - v[1]) * k + ")"; });
    circle.attr("r", function(d) { if (d.children) return d.r *k; if (d.data.CPULimit && d.data.CPURequest) return d.r * k; else return 20 * k ; })
    circle.style("fill", function(d) { if (d.children) return color(d.depth); if (!d.data.CPULimit || !d.data.CPURequest)return "grey"; else return color(d.depth);});
    innerleaf.attr("r", function(d) { if (d.data.CPULimit && d.data.CPURequest) {
                                        if (d.data.CPULimit == d.data.CPURequest) return d.r  * k;
                                        else return d.r * 2 *k; }});
  }
 
        let current_circle = undefined;
        function clearData(d) {
            console.log("CCLEAR DATA");
                svg.selectAll("#details-popup").remove();
        }

        function showData(d) {
            // cleanup previous selected circle
            if(current_circle !== undefined){
                svg.selectAll("#details-popup").remove();
            }
            console.log("here I am" + d.data.name);

        // select the circle
        current_circle = d3.select(this);
        console.log("here");
        console.log(current_circle);

        let textblock = svg.selectAll("#details-popup")
          .data([d])
          .enter()
          .append("g")
          .attr("id", "details-popup")
          .attr("font-size", 14)
          .attr("font-family", "sans-serif")
          .attr("text-anchor", "start")
          .attr("transform", d => `translate(0, 20)`);

        textblock.append("text")
          .text("Details:")
          .attr("font-weight", "bold");
        textblock.append("text")
          .text(d => "Name: " + d.data.name)
          .attr("y", "16");
        textblock.append("text")
          .text(d => "CPUs: " + d.data.CPUs)
          .attr("y", "32");
        textblock.append("text")
          .text(d => "CPU Request: " + d.data.CPURequest)
          .attr("y", "48");
        textblock.append("text")
          .text(d => "CPU Limit: " + d.data.CPULimit)
          .attr("y", "64");
        textblock.append("text")
          .text(d => "Memory Request: " + d.data.MemoryRequest)
          .attr("y", "80");
        textblock.append("text")
          .text(d => "Memory Limit: " + d.data.MemoryLimit)
          .attr("y", "96");
        }
    });
}

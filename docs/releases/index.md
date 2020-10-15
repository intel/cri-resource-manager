# Releases

For up-to-date user documentation see the [documentation site](/cri-resource-manager)

## Documentation for Released Versions
<div id="releases">
</div>
<script src="releases.js"></script>
<script>
  var list = document.getElementById('releases').appendChild(document.createElement("ul"));
  var releaseItems = getReleaseListItems();
  for (var i=0; i < releaseItems.length; i++) {
    var item = document.createElement('li');
    var paragraph = item.appendChild(document.createElement("p"));
    var anchor = paragraph.appendChild(document.createElement('a'));
    anchor.appendChild(document.createTextNode(releaseItems[i].name));
    anchor.href = releaseItems[i].url;
    anchor.class = "reference external";
    list.appendChild(item);
  }
</script>

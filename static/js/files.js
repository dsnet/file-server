var fileInfos = [];

function renderFileList(fileInfos, order, compare) {
	let fileList = document.getElementById("file-list").tBodies[0];

	// Clear existing tBody.
	while (fileList.lastChild) {
		fileList.removeChild(fileList.lastChild);
	}

	// Render the file list.
	for (var i = 0; i < fileInfos.length; i++) {
		let file = fileInfos[i];

		let tr = document.createElement("tr");

		let td1 = document.createElement("td");
		let a = document.createElement("a");
		a.setAttribute("href", file.name);
		a.appendChild(document.createTextNode(file.name));
		td1.appendChild(a);
		tr.appendChild(td1);

		let td2 = document.createElement("td");
		let sizeText = "";
		if (!file.name.endsWith("/")) {
			sizeText = formatSize(file.size);
		}
		td2.appendChild(document.createTextNode(sizeText));
		tr.appendChild(td2);

		let td3 = document.createElement("td");
		td3.appendChild(document.createTextNode(formatDate(file.date)));
		tr.appendChild(td3);

		fileList.append(tr);
	}
}

function compareNames(x, y) {
	if      (x.name < y.name) { return -1; }
	else if (x.name > y.name) { return +1; }
	else                      { return 0; }
}

function compareSizes(x, y) {
	if      (x.size < y.size) { return -1; }
	else if (x.size > y.size) { return +1; }
	else                      { return compareNames(x, y); }
}

function compareDates(x, y) {
	if      (x.date < y.date) { return -1; }
	else if (x.date > y.date) { return +1; }
	else                      { return compareNames(x, y); }
}

var order = 0;                          // -1 or +1
var orderBy = function() { return 0; }; // compareNames, compareSizes, or compareDates

function reorderFiles(nextOrderBy) {
	// Determine the next order and orderBy.
	if (orderBy === nextOrderBy) {
		order *= -1;
	} else {
		order = +1;
		orderBy = nextOrderBy;
	}

	// Remove the order indicator from all headers.
	let nameHdr = document.getElementById("name-column-header");
	let sizeHdr = document.getElementById("size-column-header");
	let dateHdr = document.getElementById("date-column-header");
	while (nameHdr.childNodes.length > 1) { nameHdr.removeChild(nameHdr.lastChild); }
	while (sizeHdr.childNodes.length > 1) { sizeHdr.removeChild(sizeHdr.lastChild); }
	while (dateHdr.childNodes.length > 1) { dateHdr.removeChild(dateHdr.lastChild); }

	// Inject the order indicator in the appropriate header.
	let orderText = document.createTextNode(" " + (order > 0 ? "↑" : "↓"));
	if      (orderBy === compareNames) { nameHdr.appendChild(orderText); }
	else if (orderBy === compareSizes) { sizeHdr.appendChild(orderText); }
	else if (orderBy === compareDates) { dateHdr.appendChild(orderText); }

	// Sort and render the file listing.
	fileInfos.sort(function(x, y) { return order * orderBy(x, y) });
	renderFileList(fileInfos);
}

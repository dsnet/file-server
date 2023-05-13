var fileInfos = [];
var numSelectedFiles = 0;

// selectAllFiles selects (or unselects) all files.
// This is called when the header checkbox is clicked.
function selectAllFiles() {
	let selectAll = document.getElementById("select-all-files").checked;
	let fileList = document.getElementById("file-list").tBodies[0];
	numSelectedFiles = 0;
	for (var i = 0; i < fileList.children.length; i++) {
		fileList.children[i].children[0].children[0].checked = selectAll; // tbody -> tr -> td -> input
		if (selectAll) { numSelectedFiles++; }
	}
	if (numSelectedFiles > fileList.children.length) {
		console.log("BUG: more selected files than total number of files");
	}
}

// updateSelectAllFiles visually updates the checkbox state of the header checkbox.
function updateSelectAllFiles() {
    let allSelected = numSelectedFiles == fileInfos.length && fileInfos.length > 0;
	document.getElementById("select-all-files").checked = allSelected;
}

// selectedFiles returns a JSON object set of all selected files.
function selectedFiles() {
	let fileList = document.getElementById("file-list").tBodies[0];
	let files = {};
	let numFiles = 0;
	for (var i = 0; i < fileList.children.length; i++) {
		let row = fileList.children[i];                  // tbody -> tr
		if (row.children[0].children[0].checked) {       // tr -> td -> input -> checked
			let name = row.children[1].children[0].text; // tr -> td -> a -> text
			files[name] = true;
			numFiles++;
		}
	}
	if (numFiles != numSelectedFiles) {
		console.log("BUG: inconsistent number of selected files")
	}
	return files;
}

// renderFileList clears and re-renders the file listing.
// It depends on the fileInfos and numSelectedFiles global variables.
function renderFileList() {
	let fileList = document.getElementById("file-list").tBodies[0];

	// Clear existing tBody and remember which were selected.
	let selectFiles = selectedFiles();
	while (fileList.lastChild) {
		fileList.removeChild(fileList.lastChild);
	}

	// Render the file list.
	numSelectedFiles = 0;
	for (var i = 0; i < fileInfos.length; i++) {
		let file = fileInfos[i];

		let tr = document.createElement("tr");

		let td0 = document.createElement("td");
		let input = document.createElement("input");
		input.setAttribute("type", "checkbox");
		if (selectFiles[file.name]) {
			input.checked = true;
			numSelectedFiles++;
		}
		input.onclick = function() {
			if (input.checked) {
				numSelectedFiles++;
			} else {
				numSelectedFiles--;
			}
			updateSelectAllFiles();
		}
		td0.appendChild(input);
		tr.appendChild(td0);

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
	updateSelectAllFiles();
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

var order = 0;
var orderBy = function() {};

// reorderFiles reorders the file listing according to some dimension.
// It depends on the order and orderBy global variables.
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
	renderFileList();
}

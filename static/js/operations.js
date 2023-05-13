var numPendingOperations = 0;
var numSelectedOperations = 0; // invariant: never greater than numFinishedOperations
var numFinishedOperations = 0;

// startOperation starts a new operation with the provided label.
// It returns a function callback that can be used to update the status.
// The fist argument is a status text, while the second is whether the
// operation has transitioned to a "done" state.
// It depends on the numXXXOperation global variables.
function startOperation(op) {
	// Show the operations pane since at least one operation is running.
	document.getElementById("operations-div").style.display = "";

	// Append a new row for the operation.
	let opsList = document.getElementById("operations-list").tBodies[0];

	let tr = document.createElement("tr");

	let td0 = document.createElement("td");
	let input = document.createElement("input");
	input.setAttribute("type", "checkbox");
	input.disabled = true; // while pending
	td0.appendChild(input);
	tr.appendChild(td0);

	let td1 = document.createElement("td");
	td1.appendChild(document.createTextNode(op));
	tr.appendChild(td1);

	let td2 = document.createElement("td");
	td2.appendChild(document.createTextNode("Pending"));
	tr.appendChild(td2);

	opsList.append(tr);

	// Construct a closured function to update the operation status.
	numPendingOperations++;
	updateSelectAllOperations();
	let pending = true;
	return function(status, done) {
		td2.firstChild.textContent = status;
		if (done) {
			if (pending) {
				numPendingOperations--;
				numFinishedOperations++;
				input.disabled = false;
				input.onclick = function() {
					if (input.checked) {
						numSelectedOperations++;
					} else {
						numSelectedOperations--;
					}
					updateSelectAllOperations();
				}
				updateSelectAllOperations();
				pending = false;
			} else {
				console.log("BUG: cannot transition from done to undone")
			}
		}
	}
}

// selectAllOperations selects (or unselects) all finished operations.
// This is called when the header checkbox is clicked.
function selectAllOperations() {
	let selectAll = document.getElementById("select-all-operations").checked;
	let opsList = document.getElementById("operations-list").tBodies[0];
	numSelectedOperations = 0;
	for (var i = 0; i < opsList.children.length; i++) {
		let input = opsList.children[i].children[0].children[0]; // tbody -> tr -> td -> input
		if (!input.disabled) {
			input.checked = selectAll;
			if (selectAll) { numSelectedOperations++; }
		}
	}
	if (numSelectedOperations > numFinishedOperations) {
		console.log("BUG: more selected operations than finished operations");
	}
	if (numPendingOperations+numFinishedOperations != opsList.children.length) {
		console.log("BUG: inconsistent number of operations");
	}
}

// updateSelectAllOperations visually updates the checkbox state of the header checkbox.
function updateSelectAllOperations() {
	let allSelected = numSelectedOperations == numFinishedOperations && numFinishedOperations > 0;
	document.getElementById("select-all-operations").checked = allSelected;
}

// hideSelectedOperations hides all selected operations.
// this is called when the "Hide" button is called.
function hideSelectedOperations() {
	let opsList = document.getElementById("operations-list").tBodies[0];

	// Remove operation entries that are selected.
	for (var i = 0; i < opsList.children.length; i++) {
		let input = opsList.children[i].children[0].children[0]; // tBody -> tr -> td -> input
		if (input.checked) {
			opsList.children[i].remove();
			numSelectedOperations--;
			numFinishedOperations--;
			i--;
		}
	}
	updateSelectAllOperations();

	// Hide the operations pane since no operation remains.
	if (numPendingOperations+numFinishedOperations == 0) {
		document.getElementById("operations-div").style.display = "none";
	}

	if (numSelectedOperations > 0) {
		console.log("BUG: remaining selected operations after hiding");
	}
	if (numPendingOperations+numFinishedOperations != opsList.children.length) {
		console.log("BUG: inconsistent number of operations");
	}
}

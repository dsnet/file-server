function formatSize(size) {
	let units = "=KMGTPEZY";
	while (size >= 1024) {
		size /= 1024;
		units = units.slice(1);
	}
	if (units[0] == '=') {
		return size.toFixed(0)+"B";
	} else {
		return size.toFixed(1)+units.slice(0,1)+"iB";
	}
}

function formatDate(unix) {
	let delta = Date.now()/1e3 - unix;
	let date = new Date(unix*1e3);
	if (-12*60*60 < delta && delta < +12*60*60) {
		let label = "AM", offset = 0;
		if (date.getHours() > 12) {
			label = "PM", offset = 12;
		}
		return String(date.getHours()-offset) + ":" + String(date.getMinutes()).padStart(2, '0') + " " + label;
	} else {
		const month = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
		return month[date.getMonth()] + " " + String(date.getDate()) + ", " + String(date.getFullYear());
	}
}

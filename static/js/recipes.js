// static/js/recipes.js
// Instant client-side search + tag filtering for /recipes. No server
// round trip: the whole collection is already in the rendered DOM, and
// a personal recipe list is small enough that this is plenty fast.
(function () {
	var searchInput = document.getElementById("recipe-search");
	var tagSelect = document.getElementById("recipe-tag-filter");
	var cards = document.querySelectorAll(".recipe-card");

	function matchesTag(card) {
		var activeTag = tagSelect ? tagSelect.value : "";
		if (!activeTag) return true;
		var tags = (card.getAttribute("data-tags") || "").split("|");
		return tags.indexOf(activeTag) !== -1;
	}

	function matchesQuery(card, query) {
		if (!query) return true;
		var text = card.getAttribute("data-search") || "";
		return text.indexOf(query) !== -1;
	}

	function applyFilter() {
		var query = (searchInput ? searchInput.value : "").toLowerCase().trim();
		cards.forEach(function (card) {
			card.hidden = !(matchesQuery(card, query) && matchesTag(card));
		});
	}

	if (searchInput) {
		searchInput.addEventListener("input", applyFilter);
	}

	if (tagSelect) {
		tagSelect.addEventListener("change", applyFilter);
	}
})();

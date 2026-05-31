(function () {
  function addCard() {
    var row = document.querySelector('#custom-cards .row');
    if (!row) return;
    var col = document.createElement('div');
    col.className = 'col';
    col.innerHTML =
      '<a href="/yaamon/" class="link-underline link-underline-opacity-0">' +
        '<div class="card card-cover h-100 overflow-hidden text-bg-dark rounded-4 shadow-lg"' +
            ' style="background-image: url(\'/img/allmon3.jpg\');">' +
          '<div class="d-flex flex-column h-100 p-5 pb-3 text-white text-shadow-1">' +
            '<h3 class="pt-5 mt-5 mb-4 display-6 lh-1 fw-bold">YAAMon</h3>' +
            '<p>Yet Another AllStar MONitor (and favorites)</p>' +
          '</div>' +
        '</div>' +
      '</a>';
    row.appendChild(col);
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', addCard);
  } else {
    addCard();
  }
})();

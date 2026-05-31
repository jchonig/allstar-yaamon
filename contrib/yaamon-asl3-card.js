(function () {
  function addCard() {
    var row = document.querySelector('#custom-cards .row');
    if (!row) return;

    // Switch from 3-col to 4-col layout to fit the new card.
    row.classList.remove('row-cols-lg-3');
    row.classList.add('row-cols-lg-4');

    var col = document.createElement('div');
    col.className = 'col';
    col.innerHTML =
      '<a href="/yaamon/" class="link-underline link-underline-opacity-0">' +
        '<div class="card card-cover h-100 overflow-hidden text-bg-dark rounded-4 shadow-lg"' +
            ' style="background-image: url(\'/img/allmon3.jpg\');">' +
          '<div class="d-flex flex-column h-100 p-3 pb-2 text-white text-shadow-1">' +
            '<h3 class="pt-4 mt-4 mb-3 display-6 lh-1 fw-bold">YAAMon</h3>' +
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

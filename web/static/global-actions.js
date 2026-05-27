(function () {
  function onReady(fn) {
    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', fn);
      return;
    }
    fn();
  }

  onReady(function () {
    var root = document.querySelector('[data-global-actions]');
    var toggle = document.querySelector('[data-global-action-toggle]');
    var menu = document.querySelector('[data-global-action-menu]');
    var backdrop = document.querySelector('[data-global-action-backdrop]');
    var drawer = document.querySelector('[data-global-action-drawer]');
    var drawerTitle = document.getElementById('global-action-drawer-title');
    var drawerContent = document.getElementById('global-action-drawer-content');
    var closeButtons = document.querySelectorAll('[data-global-action-close]');
    var lastFocus = null;

    if (!root || !toggle || !menu || !backdrop || !drawer || !drawerContent) {
      return;
    }

    function isMenuOpen() {
      return !menu.hidden;
    }

    function isDrawerOpen() {
      return !drawer.hidden;
    }

    function showBackdrop() {
      backdrop.hidden = false;
    }

    function hideBackdropIfIdle() {
      if (!isMenuOpen() && !isDrawerOpen()) {
        backdrop.hidden = true;
      }
    }

    function openMenu() {
      lastFocus = document.activeElement;
      menu.hidden = false;
      showBackdrop();
      toggle.setAttribute('aria-expanded', 'true');
    }

    function closeMenu(keepBackdrop) {
      menu.hidden = true;
      toggle.setAttribute('aria-expanded', 'false');
      if (!keepBackdrop) {
        hideBackdropIfIdle();
      }
    }

    function openDrawer() {
      drawer.hidden = false;
      showBackdrop();
      window.requestAnimationFrame(function () {
        drawer.classList.add('is-open');
      });
    }

    function closeDrawer() {
      drawer.classList.remove('is-open');
      window.setTimeout(function () {
        if (drawer.classList.contains('is-open')) {
          return;
        }
        drawer.hidden = true;
        drawerContent.replaceChildren();
        drawerContent.classList.remove('is-loading');
        hideBackdropIfIdle();
        if (lastFocus && typeof lastFocus.focus === 'function') {
          lastFocus.focus({ preventScroll: true });
        }
      }, 190);
    }

    function setLoading(choice) {
      if (drawerTitle && choice.dataset.drawerTitle) {
        drawerTitle.textContent = choice.dataset.drawerTitle;
      }
      drawerContent.classList.add('is-loading');
      drawerContent.replaceChildren();
      var loading = document.createElement('p');
      loading.className = 'muted';
      loading.textContent = 'Caricamento...';
      drawerContent.appendChild(loading);
    }

    toggle.addEventListener('click', function () {
      if (isDrawerOpen()) {
        closeDrawer();
        return;
      }
      if (isMenuOpen()) {
        closeMenu(false);
        return;
      }
      openMenu();
    });

    menu.addEventListener('click', function (event) {
      var choice = event.target.closest('[data-global-action-choice]');
      if (!choice) {
        return;
      }
      setLoading(choice);
      closeMenu(true);
      openDrawer();
    });

    backdrop.addEventListener('click', function () {
      if (isDrawerOpen()) {
        closeDrawer();
        return;
      }
      closeMenu(false);
    });

    closeButtons.forEach(function (button) {
      button.addEventListener('click', closeDrawer);
    });

    document.addEventListener('keydown', function (event) {
      if (event.key !== 'Escape') {
        return;
      }
      if (isDrawerOpen()) {
        closeDrawer();
        return;
      }
      if (isMenuOpen()) {
        closeMenu(false);
      }
    });

    document.body.addEventListener('htmx:afterSwap', function (event) {
      if (event.detail.target !== drawerContent) {
        return;
      }
      drawerContent.classList.remove('is-loading');
      var firstField = drawerContent.querySelector('input, select, textarea, button');
      if (firstField && typeof firstField.focus === 'function') {
        firstField.focus({ preventScroll: true });
      }
    });

    document.body.addEventListener('htmx:afterRequest', function (event) {
      var form = event.target.closest ? event.target.closest('[data-global-action-form]') : null;
      if (!form || !event.detail.successful) {
        return;
      }
      closeDrawer();
    });
  });
})();

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
    var status = document.querySelector('[data-app-status]');
    var page = document.querySelector('main');
    var header = document.querySelector('.topbar');
    var lastFocus = null;
    var statusTimer = null;

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

    function setPageInert(inert) {
      [header, page, root].forEach(function (element) {
        if (!element) {
          return;
        }
        if (inert) {
          element.setAttribute('inert', '');
        } else {
          element.removeAttribute('inert');
        }
      });
      document.body.classList.toggle('has-open-drawer', inert);
    }

    function announce(message) {
      if (!status || !message) {
        return;
      }
      window.clearTimeout(statusTimer);
      status.textContent = message;
      status.hidden = false;
      status.classList.add('is-visible');
      statusTimer = window.setTimeout(function () {
        status.classList.remove('is-visible');
        status.hidden = true;
      }, 4200);
    }

    function syncNavigation() {
      var section = window.location.pathname.split('/')[1];
      document.querySelectorAll('[data-nav-section]').forEach(function (link) {
        if (link.dataset.navSection === section) {
          link.setAttribute('aria-current', 'page');
        } else {
          link.removeAttribute('aria-current');
        }
      });
    }

    function openMenu() {
      lastFocus = document.activeElement;
      menu.hidden = false;
      showBackdrop();
      toggle.setAttribute('aria-expanded', 'true');
      var firstChoice = menu.querySelector('[role="menuitem"]');
      if (firstChoice) {
        firstChoice.focus({ preventScroll: true });
      }
    }

    function closeMenu(keepBackdrop, restoreFocus) {
      menu.hidden = true;
      toggle.setAttribute('aria-expanded', 'false');
      if (!keepBackdrop) {
        hideBackdropIfIdle();
      }
      if (restoreFocus) {
        toggle.focus({ preventScroll: true });
      }
    }

    function openDrawer() {
      drawer.hidden = false;
      showBackdrop();
      setPageInert(true);
      window.requestAnimationFrame(function () {
        drawer.classList.add('is-open');
        if (drawerTitle) {
          drawerTitle.focus({ preventScroll: true });
        }
      });
    }

    function closeDrawer() {
      drawer.classList.remove('is-open');
      setPageInert(false);
      window.setTimeout(function () {
        if (drawer.classList.contains('is-open')) {
          return;
        }
        drawer.hidden = true;
        drawerContent.replaceChildren();
        drawerContent.classList.remove('is-loading');
        drawerContent.removeAttribute('aria-busy');
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
      drawerContent.setAttribute('aria-busy', 'true');
      drawerContent.replaceChildren();
      var loading = document.createElement('p');
      loading.className = 'muted';
      loading.setAttribute('role', 'status');
      loading.textContent = 'Caricamento...';
      drawerContent.appendChild(loading);
    }

    function openChoice(choice) {
      setLoading(choice);
      closeMenu(true, false);
      openDrawer();
    }

    function requestElement(event) {
      return event.detail && event.detail.elt ? event.detail.elt : event.target;
    }

    function requestForm(event) {
      var element = requestElement(event);
      return element && element.closest ? element.closest('[data-global-action-form]') : null;
    }

    function isFormSubmission(event) {
      var element = requestElement(event);
      return element && element.matches && element.matches('form[data-global-action-form]');
    }

    function setCustomValue(select, field, name, active) {
      var input = field ? field.querySelector('input') : null;
      if (!input) {
        return;
      }
      field.hidden = !active;
      input.disabled = !active;
      input.required = active;
      if (active) {
        select.removeAttribute('name');
        input.name = name;
      } else {
        select.name = name;
        input.removeAttribute('name');
        input.value = '';
      }
    }

    function syncTransactionSubcategories(form, reset) {
      var kind = form.querySelector('[name="kind"]');
      var category = form.querySelector('[data-transaction-category]');
      var select = form.querySelector('[data-transaction-subcategory]');
      var field = form.querySelector('[data-transaction-subcategory-field]');
      var customField = form.querySelector('[data-transaction-new-subcategory]');
      if (!kind || !category || !select || !field || !customField) {
        return;
      }

      var categoryValue = category.value;
      var visible = categoryValue !== '';
      field.hidden = !visible;
      select.disabled = !visible;
      select.querySelectorAll('[data-transaction-kind]').forEach(function (option) {
        var active = option.dataset.transactionKind === kind.value &&
          option.dataset.transactionCategory === categoryValue;
        option.hidden = !active;
        option.disabled = !active;
      });

      if (reset || !visible) {
        select.value = '';
      }
      var selected = select.options[select.selectedIndex];
      if (selected && selected.dataset.transactionKind && selected.disabled) {
        select.value = '';
      }

      var custom = visible && select.value === '__new__';
      setCustomValue(select, customField, 'subcategory', custom);
      if (!visible) {
        select.disabled = true;
      }
    }

    function syncTransactionCategories(form, reset) {
      var kind = form.querySelector('[name="kind"]');
      var select = form.querySelector('[data-transaction-category]');
      var customField = form.querySelector('[data-transaction-new-category]');
      if (!kind || !select || !customField) {
        return;
      }

      select.querySelectorAll('[data-transaction-kind]').forEach(function (option) {
        var active = option.dataset.transactionKind === kind.value;
        option.hidden = !active;
        option.disabled = !active;
      });
      if (reset) {
        select.value = '';
      }
      var selected = select.options[select.selectedIndex];
      if (selected && selected.dataset.transactionKind && selected.disabled) {
        select.value = '';
      }

      setCustomValue(select, customField, 'category', select.value === '__new__');
      syncTransactionSubcategories(form, true);
    }

    function transactionChoice(form, name) {
      var field = form.elements[name];
      return field && typeof field.value === 'string' ? field.value.trim().toLowerCase() : '';
    }

    function payeeContextRank(option, selected) {
      var contexts;
      try {
        contexts = JSON.parse(option.dataset.payeeContexts || '[]');
      } catch (_error) {
        contexts = [];
      }

      var bestScore = 0;
      var contextCount = 0;
      contexts.forEach(function (context) {
        var score = 0;
        if (selected.kind && String(context.kind).toLowerCase() === selected.kind) {
          score += 8;
        }
        if (selected.account && String(context.account).toLowerCase() === selected.account) {
          score += 4;
        }
        if (selected.category && String(context.category).toLowerCase() === selected.category) {
          score += 2;
        }
        if (selected.subcategory && String(context.subcategory).toLowerCase() === selected.subcategory) {
          score += 1;
        }
        if (score > bestScore) {
          bestScore = score;
          contextCount = Number(context.count) || 0;
        } else if (score === bestScore) {
          contextCount += Number(context.count) || 0;
        }
      });
      return { score: bestScore, contextCount: contextCount };
    }

    function syncTransactionPayees(form) {
      var datalist = form.querySelector('[data-transaction-payee-suggestions]');
      if (!datalist) {
        return;
      }
      var selected = {
        kind: transactionChoice(form, 'kind'),
        account: transactionChoice(form, 'account'),
        category: transactionChoice(form, 'category'),
        subcategory: transactionChoice(form, 'subcategory')
      };
      var options = Array.prototype.slice.call(datalist.querySelectorAll('option'));
      options.sort(function (left, right) {
        var leftRank = payeeContextRank(left, selected);
        var rightRank = payeeContextRank(right, selected);
        if (leftRank.score !== rightRank.score) {
          return rightRank.score - leftRank.score;
        }
        if (leftRank.contextCount !== rightRank.contextCount) {
          return rightRank.contextCount - leftRank.contextCount;
        }
        var totalDifference = Number(right.dataset.payeeTotal) - Number(left.dataset.payeeTotal);
        if (totalDifference) {
          return totalDifference;
        }
        var leftName = left.value.toLowerCase();
        var rightName = right.value.toLowerCase();
        if (leftName !== rightName) {
          return leftName < rightName ? -1 : 1;
        }
        return left.value < right.value ? -1 : left.value > right.value ? 1 : 0;
      });
      options.forEach(function (option) {
        datalist.appendChild(option);
      });
    }

    function initializeTransactionForm(form) {
      if (form && form.querySelector('[data-transaction-category]')) {
        syncTransactionCategories(form, false);
        syncTransactionPayees(form);
      }
    }

    function syncTransferAccounts(form) {
      var source = form.querySelector('[data-transfer-source]');
      var destination = form.querySelector('[data-transfer-destination]');
      if (!source || !destination) {
        return;
      }

      if (destination.value === source.value) {
        destination.value = '';
      }
      destination.querySelectorAll('option[value]:not([value=""])').forEach(function (option) {
        var matchesSource = option.value === source.value;
        option.hidden = matchesSource;
        option.disabled = matchesSource;
      });
    }

    function initializeActionForm(form) {
      initializeTransactionForm(form);
      if (form) {
        syncTransferAccounts(form);
      }
    }

    function setFormPending(form, pending) {
      if (!form) {
        return;
      }
      var submit = form.querySelector('button[type="submit"]');
      form.setAttribute('aria-busy', pending ? 'true' : 'false');
      if (!submit) {
        return;
      }
      if (pending) {
        submit.dataset.label = submit.textContent;
        submit.textContent = 'Salvataggio...';
        submit.disabled = true;
      } else {
        submit.textContent = submit.dataset.label || submit.textContent;
        delete submit.dataset.label;
        submit.disabled = false;
      }
    }

    function showFormError(form, message) {
      if (!form) {
        drawerContent.classList.remove('is-loading');
        drawerContent.removeAttribute('aria-busy');
        drawerContent.replaceChildren();
        var loadError = document.createElement('p');
        loadError.className = 'form-error';
        loadError.setAttribute('role', 'alert');
        loadError.textContent = message || 'Impossibile caricare il modulo. Riprova.';
        drawerContent.appendChild(loadError);
        return;
      }
      var error = form.querySelector('[data-form-error]');
      if (!error) {
        return;
      }
      error.textContent = message || 'Non è stato possibile salvare. Riprova.';
      error.hidden = false;
      error.focus({ preventScroll: true });
    }

    function focusableElements() {
      return Array.prototype.slice.call(drawer.querySelectorAll(
        'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
      )).filter(function (element) {
        return !element.hidden && element.offsetParent !== null;
      });
    }

    toggle.addEventListener('click', function () {
      if (isDrawerOpen()) {
        closeDrawer();
        return;
      }
      if (isMenuOpen()) {
        closeMenu(false, true);
        return;
      }
      openMenu();
    });

    menu.addEventListener('click', function (event) {
      var choice = event.target.closest('[data-global-action-choice]');
      if (choice) {
        openChoice(choice);
      }
    });

    menu.addEventListener('keydown', function (event) {
      var choices = Array.prototype.slice.call(menu.querySelectorAll('[role="menuitem"]'));
      var current = choices.indexOf(document.activeElement);
      var next = current;
      if (event.key === 'ArrowDown') {
        next = (current + 1) % choices.length;
      } else if (event.key === 'ArrowUp') {
        next = (current - 1 + choices.length) % choices.length;
      } else if (event.key === 'Home') {
        next = 0;
      } else if (event.key === 'End') {
        next = choices.length - 1;
      } else {
        return;
      }
      event.preventDefault();
      choices[next].focus();
    });

    drawerContent.addEventListener('change', function (event) {
      var form = event.target.closest('[data-global-action-form]');
      if (!form) {
        return;
      }
      if (event.target.matches('[data-transfer-source]')) {
        syncTransferAccounts(form);
      } else if (event.target.matches('[name="kind"]')) {
        syncTransactionCategories(form, true);
      } else if (event.target.matches('[data-transaction-category]')) {
        syncTransactionCategories(form, false);
      } else if (event.target.matches('[data-transaction-subcategory]')) {
        syncTransactionSubcategories(form, false);
      }
      if (event.target.matches('[name="kind"], [name="account"], [name="category"], [name="subcategory"], [data-transaction-category], [data-transaction-subcategory]')) {
        syncTransactionPayees(form);
      }
    });

    document.addEventListener('click', function (event) {
      var trigger = event.target.closest('[data-open-global-action]');
      if (!trigger) {
        return;
      }
      var choice = menu.querySelector('[data-action-name="' + trigger.dataset.openGlobalAction + '"]');
      if (choice && window.htmx) {
        setLoading(choice);
        window.htmx.ajax('GET', choice.getAttribute('hx-get'), {
          target: drawerContent,
          swap: 'innerHTML'
        });
      }
    });

    backdrop.addEventListener('click', function () {
      if (isDrawerOpen()) {
        closeDrawer();
      } else {
        closeMenu(false, true);
      }
    });

    drawer.addEventListener('click', function (event) {
      if (event.target === drawer) {
        closeDrawer();
      }
    });

    closeButtons.forEach(function (button) {
      button.addEventListener('click', closeDrawer);
    });

    document.addEventListener('keydown', function (event) {
      if (event.key === 'Escape') {
        if (isDrawerOpen()) {
          closeDrawer();
        } else if (isMenuOpen()) {
          closeMenu(false, true);
        }
        return;
      }
      if (event.key !== 'Tab' || !isDrawerOpen()) {
        return;
      }
      var focusable = focusableElements();
      if (!focusable.length) {
        event.preventDefault();
        drawerTitle.focus();
        return;
      }
      var first = focusable[0];
      var last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    });

    document.body.addEventListener('htmx:beforeRequest', function (event) {
      var form = requestForm(event);
      if (form) {
        var error = form.querySelector('[data-form-error]');
        if (error) {
          error.hidden = true;
          error.textContent = '';
        }
        if (isFormSubmission(event)) {
          setFormPending(form, true);
        }
      }
    });

    document.body.addEventListener('htmx:afterSwap', function (event) {
      if (event.detail.target !== drawerContent) {
        syncNavigation();
        return;
      }
      drawerContent.classList.remove('is-loading');
      drawerContent.removeAttribute('aria-busy');
      initializeActionForm(drawerContent.querySelector('[data-global-action-form]'));
      var firstField = drawerContent.querySelector('input, select, textarea, button');
      if (firstField && typeof firstField.focus === 'function') {
        firstField.focus({ preventScroll: true });
      }
    });

    document.body.addEventListener('htmx:beforeOnLoad', function (event) {
      var xhr = event.detail && event.detail.xhr;
      var message = xhr && xhr.getResponseHeader('X-Spese-Success');
      if (message) {
        sessionStorage.setItem('spese-success', message);
        announce(message);
      }
    });

    document.body.addEventListener('htmx:afterRequest', function (event) {
      var form = requestForm(event);
      if (!form) {
        return;
      }
      if (!isFormSubmission(event)) {
        return;
      }
      setFormPending(form, false);
      if (event.detail.successful) {
        closeDrawer();
      }
    });

    document.body.addEventListener('htmx:responseError', function (event) {
      var xhr = event.detail && event.detail.xhr;
      showFormError(requestForm(event), xhr && xhr.responseText ? xhr.responseText.trim() : 'Non è stato possibile completare la richiesta.');
    });

    document.body.addEventListener('htmx:sendError', function (event) {
      showFormError(requestForm(event), 'Connessione non disponibile. Controlla la rete e riprova.');
    });

    window.addEventListener('popstate', syncNavigation);
    syncNavigation();
    var savedMessage = sessionStorage.getItem('spese-success');
    if (savedMessage) {
      sessionStorage.removeItem('spese-success');
      announce(savedMessage);
    }
  });
})();

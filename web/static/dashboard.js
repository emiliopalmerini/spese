/**
 * Dashboard JavaScript
 * Handles FAB speed dial, bottom sheet, and accordion
 */

document.addEventListener('DOMContentLoaded', () => {
  initFAB();
  initBottomSheet();
  initAccordion();
});

// ============================================================
// FAB Speed Dial
// ============================================================
function initFAB() {
  const fab = document.getElementById('fab');
  const fabActions = document.getElementById('fabActions');
  const fabBackdrop = document.getElementById('fabBackdrop');

  if (!fab) return;

  let isOpen = false;
  let fabHidden = false;
  let lastScrollY = 0;

  function openFAB() {
    isOpen = true;
    fab.classList.add('fab--open');
    fabActions.classList.add('fab-actions--open');
    fabBackdrop.classList.add('fab-backdrop--open');
  }

  function closeFAB() {
    isOpen = false;
    fab.classList.remove('fab--open');
    fabActions.classList.remove('fab-actions--open');
    fabBackdrop.classList.remove('fab-backdrop--open');
  }

  fab.addEventListener('click', () => {
    if (isOpen) {
      closeFAB();
    } else {
      openFAB();
    }
  });

  fabBackdrop.addEventListener('click', closeFAB);

  // Handle action clicks
  document.querySelectorAll('.fab-action').forEach(action => {
    action.addEventListener('click', () => {
      const type = action.dataset.type;
      closeFAB();
      openBottomSheet(type);
    });
  });

  // Scroll behavior - hide on scroll down, show on scroll up
  window.addEventListener('scroll', () => {
    const currentScrollY = window.scrollY;

    if (currentScrollY > lastScrollY && currentScrollY > 100 && !fabHidden) {
      fab.classList.add('fab--hidden');
      fabHidden = true;
      if (isOpen) closeFAB();
    } else if (currentScrollY < lastScrollY && fabHidden) {
      fab.classList.remove('fab--hidden');
      fabHidden = false;
    }

    lastScrollY = currentScrollY;
  }, { passive: true });
}

// ============================================================
// Bottom Sheet
// ============================================================
let currentSheetType = null;

function initBottomSheet() {
  const backdrop = document.getElementById('sheetBackdrop');
  const closeBtn = document.getElementById('sheetClose');

  if (backdrop) {
    backdrop.addEventListener('click', closeBottomSheet);
  }

  if (closeBtn) {
    closeBtn.addEventListener('click', closeBottomSheet);
  }

  // Listen for successful form submissions
  document.body.addEventListener('htmx:afterRequest', (event) => {
    const target = event.detail.target;
    if (target && target.closest('.bottom-sheet__content')) {
      const xhr = event.detail.xhr;
      if (xhr && xhr.status >= 200 && xhr.status < 300) {
        const trigger = xhr.getResponseHeader('HX-Trigger');
        if (trigger && (trigger.includes('dashboard:refresh') || trigger.includes('show-notification'))) {
          closeBottomSheet();
          const fab = document.getElementById('fab');
          if (fab) {
            fab.classList.add('fab--success');
            setTimeout(() => fab.classList.remove('fab--success'), 600);
          }
        }
      }
    }
  });
}

function openBottomSheet(type) {
  const backdrop = document.getElementById('sheetBackdrop');
  const sheet = document.getElementById('bottomSheet');
  const title = document.getElementById('sheetTitle');
  const content = document.getElementById('sheetContent');

  currentSheetType = type;

  const titles = {
    'expense': 'Nuova Spesa',
    'income': 'Nuova Entrata',
    'recurring': 'Nuova Ricorrente'
  };
  title.textContent = titles[type] || 'Nuovo';

  const formUrls = {
    'expense': '/ui/form/expense',
    'income': '/ui/form/income',
    'recurring': '/ui/form/recurring'
  };

  content.innerHTML = '<div class="skeleton" style="height: 300px;"></div>';

  htmx.ajax('GET', formUrls[type], {
    target: content,
    swap: 'innerHTML'
  });

  backdrop.classList.add('bottom-sheet-backdrop--open');
  sheet.classList.add('bottom-sheet--open');
  document.body.style.overflow = 'hidden';
}

function closeBottomSheet() {
  const backdrop = document.getElementById('sheetBackdrop');
  const sheet = document.getElementById('bottomSheet');

  backdrop.classList.remove('bottom-sheet-backdrop--open');
  sheet.classList.remove('bottom-sheet--open');
  document.body.style.overflow = '';
  currentSheetType = null;
}

// ============================================================
// Accordion
// ============================================================
function initAccordion() {
  document.querySelectorAll('.accordion__trigger').forEach(trigger => {
    trigger.addEventListener('click', () => {
      const accordion = trigger.closest('.accordion');
      accordion.classList.toggle('accordion--open');
    });
  });
}

// ============================================================
// Toast Notifications
// ============================================================
function showToast(message, type = 'info', duration = 3000) {
  const container = document.getElementById('toastContainer');
  if (!container) return;

  const toast = document.createElement('div');
  toast.className = `toast toast--${type}`;
  toast.innerHTML = `<span class="toast__message">${message}</span>`;
  container.appendChild(toast);

  setTimeout(() => {
    toast.classList.add('toast--out');
    setTimeout(() => toast.remove(), 200);
  }, duration);
}

document.body.addEventListener('show-notification', (event) => {
  const { type, message, duration } = event.detail;
  showToast(message, type, duration || 3000);
});

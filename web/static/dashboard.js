/**
 * Dashboard JavaScript
 * Handles FAB speed dial, bottom sheet, charts, and accordion
 */

document.addEventListener('DOMContentLoaded', () => {
  initFAB();
  initBottomSheet();
  initAccordion();
  initPeriodChips();
  initCharts();
});

// ============================================================
// FAB Speed Dial
// ============================================================
function initFAB() {
  const fab = document.getElementById('fab');
  const fabContainer = document.getElementById('fabContainer');
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
        // Check for success trigger in response headers
        const trigger = xhr.getResponseHeader('HX-Trigger');
        if (trigger && (trigger.includes('dashboard:refresh') || trigger.includes('show-notification'))) {
          closeBottomSheet();
          // Pulse FAB on success
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

  // Set title based on type
  const titles = {
    'expense': 'Nuova Spesa',
    'income': 'Nuova Entrata',
    'recurring': 'Nuova Ricorrente'
  };
  title.textContent = titles[type] || 'Nuovo';

  // Load form via HTMX
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

  // Open sheet
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
// Period Chips
// ============================================================
let currentPeriod = 'month';

function initPeriodChips() {
  const chips = document.querySelectorAll('.period-chip');

  chips.forEach(chip => {
    chip.addEventListener('click', () => {
      // Update active state
      chips.forEach(c => c.classList.remove('period-chip--active'));
      chip.classList.add('period-chip--active');

      // Update current period and refresh charts
      currentPeriod = chip.dataset.period;
      renderCharts();
    });
  });
}

// ============================================================
// Charts
// ============================================================
let trendChart = null;
let categoryChart = null;

function initCharts() {
  // Wait for Chart.js to load
  if (typeof Chart === 'undefined') {
    setTimeout(initCharts, 100);
    return;
  }

  renderCharts();

  // Re-render on dashboard refresh
  document.body.addEventListener('dashboard:refresh', renderCharts);
}

async function renderCharts() {
  try {
    const [trendData, categoryData] = await Promise.all([
      fetch(`/api/dashboard/trend?period=${currentPeriod}`).then(r => r.json()),
      fetch(`/api/dashboard/categories?period=${currentPeriod}`).then(r => r.json())
    ]);

    renderTrendChart(trendData);
    renderCategoryChart(categoryData);
  } catch (error) {
    console.error('Error loading chart data:', error);
  }
}

function renderTrendChart(data) {
  const ctx = document.getElementById('trendChart');
  if (!ctx) return;

  // Handle null or empty data
  if (!data || !Array.isArray(data) || data.length === 0) {
    data = [];
  }

  const labels = data.map(d => d.date);
  const values = data.map(d => d.amount / 100); // Convert cents to euros

  const config = {
    type: 'line',
    data: {
      labels,
      datasets: [{
        label: 'Spese',
        data: values,
        borderColor: 'rgb(17, 17, 17)',
        backgroundColor: 'rgba(17, 17, 17, 0.05)',
        tension: 0.3,
        fill: true,
        pointRadius: 0,
        pointHoverRadius: 4
      }]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      interaction: { mode: 'nearest', intersect: false },
      plugins: {
        legend: { display: false },
        tooltip: {
          backgroundColor: 'rgb(17, 17, 17)',
          titleFont: { size: 12 },
          bodyFont: { size: 14, weight: 'bold' },
          padding: 10,
          cornerRadius: 8,
          callbacks: {
            label: (ctx) => `€${ctx.formattedValue}`
          }
        }
      },
      scales: {
        y: {
          beginAtZero: true,
          grace: '10%',
          grid: { color: 'rgba(0,0,0,0.05)' },
          ticks: {
            font: { size: 11 },
            callback: (value) => `€${value}`
          }
        },
        x: {
          grid: { display: false },
          ticks: { font: { size: 11 }, maxTicksLimit: 6 }
        }
      }
    }
  };

  if (trendChart) {
    trendChart.data = config.data;
    trendChart.options = config.options;
    trendChart.update('none');
  } else {
    trendChart = new Chart(ctx, config);
  }
}

function renderCategoryChart(data) {
  const ctx = document.getElementById('categoryChart');
  if (!ctx) return;

  // Handle null or empty data
  if (!data || !Array.isArray(data) || data.length === 0) {
    data = [];
  }

  // Colors for categories
  const colors = [
    '#111111', '#4F46E5', '#059669', '#DC2626', '#F59E0B',
    '#8B5CF6', '#EC4899', '#06B6D4', '#84CC16', '#F97316'
  ];

  const labels = data.map(d => d.name);
  const values = data.map(d => d.amount / 100);
  const backgroundColors = data.map((_, i) => colors[i % colors.length]);

  const config = {
    type: 'doughnut',
    data: {
      labels,
      datasets: [{
        data: values,
        backgroundColor: backgroundColors,
        borderWidth: 0,
        hoverOffset: 4
      }]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      cutout: '60%',
      plugins: {
        legend: {
          position: 'bottom',
          labels: {
            boxWidth: 12,
            padding: 8,
            font: { size: 11 }
          }
        },
        tooltip: {
          backgroundColor: 'rgb(17, 17, 17)',
          padding: 10,
          cornerRadius: 8,
          callbacks: {
            label: (ctx) => {
              const total = ctx.dataset.data.reduce((a, b) => a + b, 0);
              const percentage = total > 0 ? ((ctx.raw / total) * 100).toFixed(1) : 0;
              return `€${ctx.formattedValue} (${percentage}%)`;
            }
          }
        }
      }
    }
  };

  if (categoryChart) {
    categoryChart.data = config.data;
    categoryChart.options = config.options;
    categoryChart.update('none');
  } else {
    categoryChart = new Chart(ctx, config);
  }
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

// Listen for HTMX show-notification events
document.body.addEventListener('show-notification', (event) => {
  const { type, message, duration } = event.detail;
  showToast(message, type, duration || 3000);
});

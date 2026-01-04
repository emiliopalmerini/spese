/**
 * Spese Notification System
 * Handles dynamic notifications with auto-dismiss, icons, and progress bar
 */

class NotificationManager {
    constructor() {
        this.container = null;
        this.initialized = false;
        this.icons = {
            success: '<svg viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd"/></svg>',
            error: '<svg viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd"/></svg>',
            info: '<svg viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clip-rule="evenodd"/></svg>'
        };
        this.init();
    }

    init() {
        if (document.readyState === 'loading') {
            document.addEventListener('DOMContentLoaded', () => this.setup());
        } else {
            this.setup();
        }
    }

    setup() {
        if (this.initialized) return;

        this.container = document.getElementById('notifications');
        if (!this.container) {
            console.warn('Notification container not found');
            return;
        }

        if (!this.customListenerAdded) {
            document.addEventListener('show-notification', (event) => {
                this.show(event.detail);
            });
            document.addEventListener('page:refresh', () => {
                setTimeout(() => location.reload(), 1500);
            });
            this.customListenerAdded = true;
        }

        this.observeNotifications();
        this.initialized = true;
    }

    observeNotifications() {
        const observer = new MutationObserver((mutations) => {
            mutations.forEach((mutation) => {
                mutation.addedNodes.forEach((node) => {
                    if (node.nodeType === 1 && node.classList.contains('notification')) {
                        this.setupNotification(node);
                    }
                });
            });
        });

        observer.observe(this.container, { childList: true });
    }

    setupNotification(notification) {
        const duration = parseInt(notification.dataset.autoDismiss);
        const type = notification.classList.contains('success') ? 'success'
            : notification.classList.contains('error') ? 'error' : 'info';

        // Wrap existing content if not already structured
        if (!notification.querySelector('.notification__content')) {
            const message = notification.textContent.trim();
            notification.innerHTML = '';

            // Add icon
            const iconWrapper = document.createElement('span');
            iconWrapper.className = 'notification__icon';
            iconWrapper.innerHTML = this.icons[type];
            notification.appendChild(iconWrapper);

            // Add content wrapper
            const content = document.createElement('span');
            content.className = 'notification__content';
            content.textContent = message;
            notification.appendChild(content);
        }

        // Add progress bar for auto-dismiss (only add once)
        if (duration > 0 && !notification.querySelector('.notification__progress')) {
            const progress = document.createElement('div');
            progress.className = 'notification__progress';
            progress.style.animation = `progressShrink ${duration}ms linear forwards`;
            notification.appendChild(progress);

            notification._dismissTimeout = setTimeout(() => {
                this.dismiss(notification);
            }, duration);

            // Pause on hover
            notification.addEventListener('mouseenter', () => {
                if (notification._dismissTimeout) {
                    clearTimeout(notification._dismissTimeout);
                    notification._dismissTimeout = null;
                }
            });
            notification.addEventListener('mouseleave', () => {
                const remaining = this.getRemainingTime(progress);
                if (remaining > 0) {
                    notification._dismissTimeout = setTimeout(() => {
                        this.dismiss(notification);
                    }, remaining);
                }
            });
        }

        // Click to dismiss (only add once)
        if (!notification._clickHandlerAdded) {
            notification.addEventListener('click', () => {
                this.dismiss(notification);
            });
            notification._clickHandlerAdded = true;
        }

        // Close button (only add once)
        if (!notification.querySelector('.notification__close')) {
            const closeBtn = document.createElement('button');
            closeBtn.innerHTML = '<svg viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M1 1l12 12M13 1L1 13"/></svg>';
            closeBtn.className = 'notification__close';
            closeBtn.setAttribute('aria-label', 'Close notification');
            closeBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                this.dismiss(notification);
            });
            notification.appendChild(closeBtn);
        }
    }

    getRemainingTime(progress) {
        const computed = getComputedStyle(progress);
        const width = parseFloat(computed.width);
        const parentWidth = progress.parentElement.offsetWidth;
        return (width / parentWidth) * parseInt(progress.parentElement.dataset.autoDismiss);
    }

    show(options = {}) {
        if (!this.container) return;

        const { type = 'info', message, duration = 3000 } = options;

        const notification = document.createElement('div');
        notification.className = `notification ${type}`;
        notification.dataset.autoDismiss = duration;
        notification.setAttribute('role', 'alert');
        notification.setAttribute('aria-live', 'polite');

        // Add icon
        const iconWrapper = document.createElement('span');
        iconWrapper.className = 'notification__icon';
        iconWrapper.innerHTML = this.icons[type];
        notification.appendChild(iconWrapper);

        // Add content
        const content = document.createElement('span');
        content.className = 'notification__content';
        content.textContent = message;
        notification.appendChild(content);

        this.container.appendChild(notification);
        // MutationObserver will call setupNotification automatically

        return notification;
    }

    dismiss(notification) {
        if (!notification || !notification.parentNode) return;
        if (notification._dismissTimeout) {
            clearTimeout(notification._dismissTimeout);
        }

        notification.style.animation = 'slideOutNotification 0.25s ease forwards';

        setTimeout(() => {
            if (notification.parentNode) {
                notification.parentNode.removeChild(notification);
            }
        }, 250);
    }

    clear() {
        if (!this.container) return;
        this.container.innerHTML = '';
    }
}

// Initialize notification manager (only once)
if (!window.notificationManager) {
    window.notificationManager = new NotificationManager();
}

// Export for use in other scripts
window.showNotification = (options) => window.notificationManager.show(options);
/**
 * Spese Notification System
 * Handles dynamic notifications with auto-dismiss and HTMX integration
 */

class NotificationManager {
    constructor() {
        this.container = null;
        this.initialized = false;
        this.init();
    }

    init() {
        // Wait for DOM to be ready
        if (document.readyState === 'loading') {
            document.addEventListener('DOMContentLoaded', () => this.setup());
        } else {
            this.setup();
        }
    }

    setup() {
        if (this.initialized) return; // Prevent multiple initialization

        this.container = document.getElementById('notifications');
        if (!this.container) {
            console.warn('Notification container not found');
            return;
        }

        // Listen for HTMX-dispatched show-notification events (only once)
        // Note: HTMX automatically dispatches events from HX-Trigger headers,
        // so we only need this listener - not htmx:afterRequest
        if (!this.customListenerAdded) {
            document.addEventListener('show-notification', (event) => {
                this.show(event.detail);
            });
            this.customListenerAdded = true;
        }

        // Setup mutation observer to handle auto-dismiss
        this.observeNotifications();

        this.initialized = true;
    }

    observeNotifications() {
        const observer = new MutationObserver((mutations) => {
            mutations.forEach((mutation) => {
                mutation.addedNodes.forEach((node) => {
                    if (node.nodeType === 1 && node.classList.contains('notification')) {
                        this.setupAutoDismiss(node);
                    }
                });
            });
        });

        observer.observe(this.container, { childList: true });
    }

    setupAutoDismiss(notification) {
        const duration = parseInt(notification.dataset.autoDismiss);
        if (duration > 0) {
            setTimeout(() => {
                this.dismiss(notification);
            }, duration);
        }

        // Add click to dismiss
        notification.addEventListener('click', () => {
            this.dismiss(notification);
        });

        // Add close button with SVG icon (iOS compatible)
        const closeBtn = document.createElement('button');
        closeBtn.innerHTML = '<svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M1 1l10 10M11 1L1 11"/></svg>';
        closeBtn.className = 'notification__close';
        closeBtn.setAttribute('aria-label', 'Close notification');
        closeBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            this.dismiss(notification);
        });
        notification.appendChild(closeBtn);
    }

    show(options = {}) {
        if (!this.container) return;

        const { type = 'info', message, duration = 3000 } = options;
        
        // Create notification element
        const notification = document.createElement('div');
        notification.className = `notification ${type}`;
        notification.dataset.autoDismiss = duration;
        notification.textContent = message;

        // Add to container
        this.container.appendChild(notification);
        
        // Setup auto dismiss
        this.setupAutoDismiss(notification);

        return notification;
    }

    dismiss(notification) {
        if (!notification || !notification.parentNode) return;

        notification.style.opacity = '0';
        notification.style.transform = 'translateX(100%)';
        
        setTimeout(() => {
            if (notification.parentNode) {
                notification.parentNode.removeChild(notification);
            }
        }, 300); // Match CSS animation duration
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
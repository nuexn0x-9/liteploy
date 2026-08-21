/**
 * LITEPLOY — Minimal JavaScript
 * 
 * Only what HTMX cannot do: clipboard, log auto-scroll, small UI helpers.
 * No framework, no state management, no build step required.
 */

// Auto-scroll log container to bottom when new content arrives.
(function () {
    'use strict';

    // Watch log container for content changes and auto-scroll if enabled.
    function initLogAutoScroll() {
        const container = document.getElementById('log-container');
        const checkbox = document.getElementById('autoscroll');
        if (!container || !checkbox) return;

        const observer = new MutationObserver(function () {
            if (checkbox.checked) {
                container.scrollTop = container.scrollHeight;
            }
        });

        observer.observe(container, { childList: true, subtree: true, characterData: true });
    }

    // Update status badge from SSE status events.
    document.addEventListener('htmx:sseMessage', function (evt) {
        try {
            const data = JSON.parse(evt.detail.data);
            if (data.status) {
                const badge = document.getElementById('dep-status');
                if (badge) {
                    // Remove old status classes.
                    badge.className = badge.className.replace(/\bstatus-\S+/g, '');
                    badge.classList.add('status-badge', 'status-' + data.status);
                    badge.textContent = data.status;
                }
            }
        } catch (e) {
            // Not a JSON status event — it's a log line, handled by HTMX.
        }
    });

    // Clipboard copy buttons.
    document.addEventListener('click', function (e) {
        const btn = e.target.closest('[data-copy]');
        if (!btn) return;
        const text = btn.dataset.copy;
        navigator.clipboard.writeText(text).then(function () {
            const orig = btn.textContent;
            btn.textContent = 'Copied!';
            setTimeout(function () { btn.textContent = orig; }, 1500);
        });
    });

    // Run after DOM is ready.
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initLogAutoScroll);
    } else {
        initLogAutoScroll();
    }

    // HTMX: send CSRF token with all non-GET requests automatically.
    // Reads from the csrf cookie set by the server.
    document.addEventListener('htmx:configRequest', function (evt) {
        const method = (evt.detail.verb || '').toUpperCase();
        if (method !== 'GET' && method !== 'HEAD') {
            // Read from cookie.
            const csrfToken = getCookie('liteploy_csrf');
            if (csrfToken) {
                evt.detail.headers['X-CSRF-Token'] = csrfToken;
            }
        }
    });

    function getCookie(name) {
        const cookies = document.cookie.split(';');
        for (let i = 0; i < cookies.length; i++) {
            const c = cookies[i].trim();
            if (c.startsWith(name + '=')) {
                return decodeURIComponent(c.substring(name.length + 1));
            }
        }
        return '';
    }
}());

// Mobile Sidebar Toggle
document.addEventListener('DOMContentLoaded', () => {
    const toggleBtn = document.querySelector('.sidebar-toggle');
    const sidebar = document.querySelector('.sidebar');
    if (toggleBtn && sidebar) {
        toggleBtn.addEventListener('click', () => {
            sidebar.classList.toggle('open');
        });
    }
});

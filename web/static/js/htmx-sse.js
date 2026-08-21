// SSE extension for HTMX in LITEPLOY
(function () {
    'use strict';
    
    function initSSE() {
        var sseElements = document.querySelectorAll('[sse-connect]');
        sseElements.forEach(function (elt) {
            if (elt.__sse_source) return;
            var url = elt.getAttribute('sse-connect');
            if (!url) return;

            var es = new EventSource(url);
            elt.__sse_source = es;

            es.onmessage = function (e) {
                var pre = elt.querySelector('pre') || elt;
                pre.textContent += e.data + '\n';
                htmx.trigger(document, 'htmx:sseMessage', { data: e.data });
            };

            es.addEventListener('status', function (e) {
                htmx.trigger(document, 'htmx:sseMessage', { data: e.data });
            });

            es.addEventListener('done', function (e) {
                es.close();
                htmx.trigger(document, 'htmx:sseMessage', { data: e.data });
            });

            es.onerror = function () {
                // If closed or error, allow natural cleanup
            };
        });
    }

    document.addEventListener('DOMContentLoaded', initSSE);
    document.addEventListener('htmx:afterSwap', initSSE);
})();

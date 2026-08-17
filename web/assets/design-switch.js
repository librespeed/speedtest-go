/**
 * Feature switch for enabling the new LibreSpeed design
 *
 * This script checks for:
 * 1. URL parameter: ?design=new or ?design=old
 * 2. Default behavior: Shows the classic design
 *
 * Note: This script is only loaded on the root index.html
 */
(function () {
    'use strict';

    // Don't run this script if we're already on a specific design page
    const currentPath = window.location.pathname;
    if (currentPath.includes('index-classic.html') || currentPath.includes('index-modern.html')) {
        return;
    }

    // Check URL parameters first
    const urlParams = new URLSearchParams(window.location.search);
    const designParam = urlParams.get('design');

    if (designParam === 'new') {
        redirectToNewDesign();
        return;
    }

    if (designParam === 'old' || designParam === 'classic') {
        redirectToOldDesign();
        return;
    }

    // Default to modern design
    redirectToNewDesign();

    function redirectToNewDesign() {
        const currentParams = window.location.search;
        window.location.href = 'index-modern.html' + currentParams;
    }

    function redirectToOldDesign() {
        const currentParams = window.location.search;
        window.location.href = 'index-classic.html' + currentParams;
    }
})();

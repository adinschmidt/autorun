// ═══════════════════════════════════════════════════════════
// autorun - Service Manager UI
// ═══════════════════════════════════════════════════════════

const state = {
    services: [],
    filteredServices: [],
    selectedService: null,
    deleteTarget: null,
    currentScope: 'all',
    searchQuery: '',
    logSocket: null,
    platform: null,
    elevated: false
};

// ═══════════════════════════════════════════════════════════
// DOM Elements
// ═══════════════════════════════════════════════════════════

const elements = {
    platformBadge: document.getElementById('platform-badge'),
    serviceList: document.getElementById('service-list'),
    serviceCount: document.getElementById('service-count'),
    searchInput: document.getElementById('search-input'),
    scopeButtons: document.querySelectorAll('.scope-btn'),
    emptyState: document.getElementById('empty-state'),
    serviceDetail: document.getElementById('service-detail'),
    detailName: document.getElementById('detail-name'),
    detailDescription: document.getElementById('detail-description'),
    detailStatus: document.getElementById('detail-status'),
    detailScope: document.getElementById('detail-scope'),
    logContent: document.getElementById('log-content'),
    logStatus: document.getElementById('log-status'),
    controlButtons: document.querySelectorAll('.ctrl-btn'),
    toastContainer: document.getElementById('toast-container'),
    // Create service elements
    createBtn: document.getElementById('create-btn'),
    createModal: document.getElementById('create-modal'),
    createModalClose: document.getElementById('create-modal-close'),
    createForm: document.getElementById('create-form'),
    createCancel: document.getElementById('create-cancel'),
    // Delete service elements
    deleteBtn: document.getElementById('delete-btn'),
    deleteModal: document.getElementById('delete-modal'),
    deleteServiceName: document.getElementById('delete-service-name'),
    deleteCancel: document.getElementById('delete-cancel'),
    deleteConfirm: document.getElementById('delete-confirm')
};

// ═══════════════════════════════════════════════════════════
// API Functions
// ═══════════════════════════════════════════════════════════

async function api(method, path, body = null) {
    const options = {
        method,
        headers: { 'Content-Type': 'application/json' }
    };
    if (body) options.body = JSON.stringify(body);

    const response = await fetch(path, options);
    const data = await response.json();

    if (!response.ok) {
        throw new Error(data.error || 'API request failed');
    }

    return data;
}

async function fetchPlatform() {
    try {
        const data = await api('GET', '/api/platform');
        state.platform = data.platform;
        state.elevated = data.elevated;
        elements.platformBadge.textContent = data.platform;
        elements.platformBadge.classList.add('detected');

        // Show elevation warning if not running as root
        if (!data.elevated) {
            showElevationWarning();
        }
    } catch (err) {
        console.error('Failed to fetch platform:', err);
        elements.platformBadge.textContent = 'ERROR';
    }
}

function showElevationWarning() {
    const warning = document.createElement('div');
    warning.className = 'elevation-warning';
    warning.innerHTML = `
        <span class="warning-icon">!</span>
        <span class="warning-text">Running without sudo &mdash; system services are read-only</span>
    `;
    document.querySelector('.sidebar-header').appendChild(warning);
}

async function fetchServices() {
    try {
        const scopeParam = state.currentScope === 'all' ? '' : `?scope=${state.currentScope}`;
        state.services = (await api('GET', `/api/services${scopeParam}`)) || [];
        filterAndRenderServices();
    } catch (err) {
        console.error('Failed to fetch services:', err);
        showToast('Couldn\'t load services. Is the server still running?', 'error');
        elements.serviceList.innerHTML = `
            <div class="list-message error">Couldn't load services</div>
        `;
        elements.serviceCount.textContent = '0';
    }
}

const actionDone = {
    start: 'Started',
    stop: 'Stopped',
    restart: 'Restarted',
    enable: 'Enabled',
    disable: 'Disabled'
};

async function performAction(action, target = state.selectedService) {
    if (!target) return;

    const { name, scope } = target;

    try {
        setControlsLoading(true);
        await api('POST', `/api/services/${encodeURIComponent(name)}/${action}?scope=${scope}`);
        showToast(`${actionDone[action] || action} ${name}`, 'success');

        // Wait briefly for launchd/systemd to update status before refreshing
        await new Promise(resolve => setTimeout(resolve, 500));

        // Refresh services (re-renders the list with fresh statuses)
        await fetchServices();

        // Update the detail view without reconnecting the log stream
        if (state.selectedService) {
            const updated = state.services.find(
                s => s.name === state.selectedService.name && s.scope === state.selectedService.scope
            );
            if (updated) {
                state.selectedService = updated;
                elements.detailStatus.className = `status-indicator ${updated.status}`;
            }
        }
    } catch (err) {
        console.error(`Action ${action} failed:`, err);
        showToast(`Couldn't ${action} ${name}: ${err.message}`, 'error');
    } finally {
        // Re-enable controls, then re-apply per-button state so buttons that
        // should stay disabled (e.g. Start while running) aren't re-enabled.
        setControlsLoading(false);
        if (state.selectedService) {
            updateControlButtons(state.selectedService);
        }
    }
}

// ═══════════════════════════════════════════════════════════
// Create Service
// ═══════════════════════════════════════════════════════════

function openCreateModal() {
    elements.createModal.style.display = 'flex';
    document.getElementById('create-name').value = '';
    document.getElementById('create-program').value = '';
    document.getElementById('create-arguments').value = '';
    document.getElementById('create-description').value = '';
    document.getElementById('create-workdir').value = '';
    document.getElementById('create-runatload').checked = true;
    document.getElementById('create-keepalive').checked = false;
    document.getElementById('create-scope').value = 'user';
    document.getElementById('create-name').focus();
}

function closeCreateModal() {
    elements.createModal.style.display = 'none';
}

async function handleCreateService(e) {
    e.preventDefault();

    const name = document.getElementById('create-name').value.trim();
    const program = document.getElementById('create-program').value.trim();
    const argumentsStr = document.getElementById('create-arguments').value.trim();
    const description = document.getElementById('create-description').value.trim();
    const workingDirectory = document.getElementById('create-workdir').value.trim();
    const runAtLoad = document.getElementById('create-runatload').checked;
    const keepAlive = document.getElementById('create-keepalive').checked;
    const scope = document.getElementById('create-scope').value;

    // Parse arguments (space-separated, respecting quotes)
    const args = argumentsStr ? parseArguments(argumentsStr) : [];

    const config = {
        name,
        program,
        arguments: args,
        description,
        workingDirectory,
        runAtLoad,
        keepAlive
    };

    try {
        await api('POST', `/api/services?scope=${scope}`, config);
        showToast(`Created ${name}`, 'success');
        closeCreateModal();
        await fetchServices();

        // Select the newly created service
        const newService = state.services.find(s => s.name === name && s.scope === scope);
        if (newService) {
            selectService(newService);
        }
    } catch (err) {
        console.error('Failed to create service:', err);
        showToast(`Couldn't create service: ${err.message}`, 'error');
    }
}

function parseArguments(str) {
    const args = [];
    let current = '';
    let inQuote = false;
    let quoteChar = '';

    for (const char of str) {
        if ((char === '"' || char === "'") && !inQuote) {
            inQuote = true;
            quoteChar = char;
        } else if (char === quoteChar && inQuote) {
            inQuote = false;
            quoteChar = '';
        } else if (char === ' ' && !inQuote) {
            if (current) {
                args.push(current);
                current = '';
            }
        } else {
            current += char;
        }
    }
    if (current) args.push(current);
    return args;
}

// ═══════════════════════════════════════════════════════════
// Delete Service
// ═══════════════════════════════════════════════════════════

function openDeleteModal(target = state.selectedService) {
    if (!target) return;
    state.deleteTarget = target;
    elements.deleteServiceName.textContent = target.name;
    elements.deleteModal.style.display = 'flex';
}

function closeDeleteModal() {
    elements.deleteModal.style.display = 'none';
    state.deleteTarget = null;
}

async function handleDeleteService() {
    const target = state.deleteTarget;
    if (!target) return;

    const { name, scope } = target;

    try {
        await api('DELETE', `/api/services/${encodeURIComponent(name)}?scope=${scope}`);
        showToast(`Deleted ${name}`, 'success');
        closeDeleteModal();

        // If the deleted service was open in the detail view, clear it
        if (state.selectedService &&
            state.selectedService.name === name && state.selectedService.scope === scope) {
            state.selectedService = null;
            elements.serviceDetail.style.display = 'none';
            elements.emptyState.style.display = 'flex';

            if (state.logSocket) {
                state.logSocket.close();
                state.logSocket = null;
            }
        }

        await fetchServices();
    } catch (err) {
        console.error('Failed to delete service:', err);
        showToast(`Couldn't delete service: ${err.message}`, 'error');
    }
}

// ═══════════════════════════════════════════════════════════
// WebSocket Log Streaming
// ═══════════════════════════════════════════════════════════

function connectLogStream(serviceName, scope) {
    // Close existing connection
    if (state.logSocket) {
        state.logSocket.close();
        state.logSocket = null;
    }

    elements.logContent.innerHTML = '<div class="log-placeholder">Connecting to logs&hellip;</div>';
    elements.logStatus.classList.remove('connected');
    elements.logStatus.innerHTML = '<span class="log-dot"></span>Connecting';

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/api/services/${encodeURIComponent(serviceName)}/logs?scope=${scope}`;

    const ws = new WebSocket(wsUrl);
    state.logSocket = ws;

    ws.onopen = () => {
        elements.logStatus.classList.add('connected');
        elements.logStatus.innerHTML = '<span class="log-dot"></span>Live';
        elements.logContent.innerHTML = '';
    };

    ws.onmessage = (event) => {
        appendLogLine(event.data);
    };

    ws.onerror = (err) => {
        console.error('WebSocket error:', err);
        elements.logStatus.classList.remove('connected');
        elements.logStatus.innerHTML = '<span class="log-dot"></span>Error';
    };

    ws.onclose = () => {
        elements.logStatus.classList.remove('connected');
        elements.logStatus.innerHTML = '<span class="log-dot"></span>Disconnected';
        state.logSocket = null;
    };
}

function appendLogLine(text) {
    const line = document.createElement('div');
    line.className = 'log-line';
    line.textContent = text;
    elements.logContent.appendChild(line);

    // Auto-scroll to bottom
    elements.logContent.scrollTop = elements.logContent.scrollHeight;

    // Limit log lines to prevent memory issues
    const maxLines = 1000;
    while (elements.logContent.children.length > maxLines) {
        elements.logContent.removeChild(elements.logContent.firstChild);
    }
}

// ═══════════════════════════════════════════════════════════
// Context Menus
// ═══════════════════════════════════════════════════════════

let contextMenu = null;
let contextTargetRow = null;

function closeContextMenu() {
    if (contextMenu) {
        contextMenu.remove();
        contextMenu = null;
    }
    if (contextTargetRow) {
        contextTargetRow.classList.remove('context-target');
        contextTargetRow = null;
    }
}

// items: {label, action, disabled?, danger?} or 'separator'
function showContextMenu(items, x, y) {
    closeContextMenu();

    const menu = document.createElement('div');
    menu.className = 'context-menu';

    for (const item of items) {
        if (item === 'separator') {
            const sep = document.createElement('div');
            sep.className = 'context-separator';
            menu.appendChild(sep);
            continue;
        }
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'context-item' + (item.danger ? ' danger' : '');
        btn.textContent = item.label;
        btn.disabled = !!item.disabled;
        btn.addEventListener('click', () => {
            closeContextMenu();
            item.action();
        });
        menu.appendChild(btn);
    }

    // Keep focus (and any text selection) where it is while clicking items
    menu.addEventListener('mousedown', (e) => e.preventDefault());

    document.body.appendChild(menu);

    // Clamp to the viewport
    const rect = menu.getBoundingClientRect();
    menu.style.left = `${Math.max(8, Math.min(x, window.innerWidth - rect.width - 8))}px`;
    menu.style.top = `${Math.max(8, Math.min(y, window.innerHeight - rect.height - 8))}px`;

    contextMenu = menu;
}

async function copyText(text) {
    try {
        await navigator.clipboard.writeText(text);
    } catch (err) {
        console.error('Clipboard write failed:', err);
        showToast("Couldn't copy to clipboard", 'error');
    }
}

function showServiceContextMenu(service, x, y) {
    const running = service.status === 'running';

    showContextMenu([
        { label: 'Start', disabled: running, action: () => performAction('start', service) },
        { label: 'Stop', disabled: !running, action: () => performAction('stop', service) },
        { label: 'Restart', disabled: !running, action: () => performAction('restart', service) },
        'separator',
        service.enabled
            ? { label: 'Disable at startup', action: () => performAction('disable', service) }
            : { label: 'Enable at startup', action: () => performAction('enable', service) },
        'separator',
        { label: 'Copy name', action: () => copyText(service.name) },
        'separator',
        { label: 'Delete…', danger: true, action: () => openDeleteModal(service) }
    ], x, y);
}

function showInputContextMenu(input, x, y) {
    input.focus();
    const start = input.selectionStart;
    const end = input.selectionEnd;
    const hasSelection = start !== end;
    const restoreSelection = () => {
        input.focus();
        input.setSelectionRange(start, end);
    };
    // Re-run listeners (e.g. live search filtering) after edits
    const notifyInput = () => input.dispatchEvent(new Event('input', { bubbles: true }));

    showContextMenu([
        {
            label: 'Cut',
            disabled: !hasSelection,
            action: () => {
                restoreSelection();
                copyText(input.value.slice(start, end));
                input.setRangeText('', start, end, 'start');
                notifyInput();
            }
        },
        {
            label: 'Copy',
            disabled: !hasSelection,
            action: () => copyText(input.value.slice(start, end))
        },
        {
            label: 'Paste',
            action: async () => {
                restoreSelection();
                try {
                    const text = await navigator.clipboard.readText();
                    input.setRangeText(text, start, end, 'end');
                    notifyInput();
                } catch (err) {
                    console.error('Clipboard read failed:', err);
                    showToast("Couldn't read the clipboard — paste with ⌘V instead", 'error');
                }
            }
        },
        'separator',
        {
            label: 'Select all',
            disabled: !input.value,
            action: () => {
                input.focus();
                input.select();
            }
        }
    ], x, y);
}

function handleContextMenu(e) {
    // This is an app, not a document: replace the browser menu everywhere
    e.preventDefault();

    const row = e.target.closest('.service-item');
    if (row) {
        const service = state.services.find(
            s => s.name === row.dataset.name && s.scope === row.dataset.scope
        );
        if (service) {
            contextTargetRow = row;
            row.classList.add('context-target');
            showServiceContextMenu(service, e.clientX, e.clientY);
        }
        return;
    }

    const input = e.target.closest('input[type="text"]');
    if (input) {
        showInputContextMenu(input, e.clientX, e.clientY);
        return;
    }

    // Selected text anywhere else (log lines, descriptions): offer Copy
    const selection = String(window.getSelection());
    if (selection) {
        showContextMenu([
            { label: 'Copy', action: () => copyText(selection) }
        ], e.clientX, e.clientY);
        return;
    }

    closeContextMenu();
}

// ═══════════════════════════════════════════════════════════
// UI Rendering
// ═══════════════════════════════════════════════════════════

function filterAndRenderServices() {
    const query = state.searchQuery.toLowerCase();

    state.filteredServices = state.services.filter(service => {
        const matchesSearch = !query || service.name.toLowerCase().includes(query);
        return matchesSearch;
    });

    // Sort: running first, then alphabetically
    state.filteredServices.sort((a, b) => {
        if (a.status === 'running' && b.status !== 'running') return -1;
        if (a.status !== 'running' && b.status === 'running') return 1;
        return a.name.localeCompare(b.name);
    });

    renderServiceList();
}

function renderServiceList() {
    if (state.filteredServices.length === 0) {
        elements.serviceList.innerHTML = `
            <div class="list-message">${state.searchQuery ? 'No services match' : 'No services here yet'}</div>
        `;
        elements.serviceCount.textContent = '0';
        return;
    }

    elements.serviceList.innerHTML = state.filteredServices.map(service => {
        const isActive = state.selectedService &&
            state.selectedService.name === service.name &&
            state.selectedService.scope === service.scope;

        return `
            <div class="service-item ${isActive ? 'active' : ''}"
                 data-name="${escapeHtml(service.name)}"
                 data-scope="${service.scope}">
                <div class="service-status ${service.status}"></div>
                <div class="service-info">
                    <div class="service-name">${escapeHtml(service.name)}</div>
                    <div class="service-scope">${service.scope}</div>
                </div>
                ${service.enabled ? '' : '<div class="service-chip" title="Doesn\'t start automatically">Manual</div>'}
            </div>
        `;
    }).join('');

    elements.serviceCount.textContent = state.filteredServices.length;

    // Add click handlers
    elements.serviceList.querySelectorAll('.service-item').forEach(item => {
        item.addEventListener('click', () => {
            const name = item.dataset.name;
            const scope = item.dataset.scope;
            const service = state.services.find(s => s.name === name && s.scope === scope);
            if (service) selectService(service);
        });
    });
}

function selectService(service) {
    state.selectedService = service;

    // Update list selection
    elements.serviceList.querySelectorAll('.service-item').forEach(item => {
        const isActive = item.dataset.name === service.name && item.dataset.scope === service.scope;
        item.classList.toggle('active', isActive);
    });

    // Show detail panel
    elements.emptyState.style.display = 'none';
    elements.serviceDetail.style.display = 'flex';

    // Update detail info
    elements.detailName.textContent = service.name;
    elements.detailDescription.textContent = service.description || 'No description';
    elements.detailStatus.className = `status-indicator ${service.status}`;
    elements.detailScope.textContent = service.scope;

    // Update control button states
    updateControlButtons(service);

    // Connect to log stream
    connectLogStream(service.name, service.scope);
}

function updateControlButtons(service) {
    const isRunning = service.status === 'running';
    const isEnabled = service.enabled;

    elements.controlButtons.forEach(btn => {
        const action = btn.dataset.action;

        switch (action) {
            case 'start':
                btn.disabled = isRunning;
                break;
            case 'stop':
            case 'restart':
                btn.disabled = !isRunning;
                break;
            case 'enable':
                btn.disabled = isEnabled;
                break;
            case 'disable':
                btn.disabled = !isEnabled;
                break;
        }
    });
}

function setControlsLoading(loading) {
    elements.controlButtons.forEach(btn => {
        btn.disabled = loading;
    });
}

// ═══════════════════════════════════════════════════════════
// Toast Notifications
// ═══════════════════════════════════════════════════════════

function showToast(message, type = 'success') {
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.textContent = message;

    elements.toastContainer.appendChild(toast);

    setTimeout(() => {
        toast.classList.add('removing');
        setTimeout(() => toast.remove(), 300);
    }, 3000);
}

// ═══════════════════════════════════════════════════════════
// Event Handlers
// ═══════════════════════════════════════════════════════════

function setupEventListeners() {
    // Scope toggle
    elements.scopeButtons.forEach(btn => {
        btn.addEventListener('click', () => {
            elements.scopeButtons.forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            state.currentScope = btn.dataset.scope;
            fetchServices();
        });
    });

    // Search
    elements.searchInput.addEventListener('input', (e) => {
        state.searchQuery = e.target.value;
        filterAndRenderServices();
    });

    // Control buttons (skip delete button - it has its own handler)
    elements.controlButtons.forEach(btn => {
        if (btn.dataset.action) {
            btn.addEventListener('click', () => {
                if (!btn.disabled) {
                    performAction(btn.dataset.action);
                }
            });
        }
    });

    // Create service
    elements.createBtn.addEventListener('click', openCreateModal);
    elements.createModalClose.addEventListener('click', closeCreateModal);
    elements.createCancel.addEventListener('click', closeCreateModal);
    elements.createForm.addEventListener('submit', handleCreateService);
    elements.createModal.addEventListener('click', (e) => {
        if (e.target === elements.createModal) closeCreateModal();
    });

    // Delete service
    elements.deleteBtn.addEventListener('click', () => openDeleteModal());
    elements.deleteCancel.addEventListener('click', closeDeleteModal);
    elements.deleteConfirm.addEventListener('click', handleDeleteService);
    elements.deleteModal.addEventListener('click', (e) => {
        if (e.target === elements.deleteModal) closeDeleteModal();
    });

    // Context menus
    document.addEventListener('contextmenu', handleContextMenu);
    document.addEventListener('click', (e) => {
        if (contextMenu && !contextMenu.contains(e.target)) closeContextMenu();
    });
    window.addEventListener('resize', closeContextMenu);
    document.addEventListener('scroll', closeContextMenu, true);

    // Keyboard shortcuts
    document.addEventListener('keydown', (e) => {
        // Focus search on Cmd/Ctrl + K
        if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
            e.preventDefault();
            elements.searchInput.focus();
        }

        // Escape to dismiss the context menu, modals, or search — in that order
        if (e.key === 'Escape') {
            if (contextMenu) {
                closeContextMenu();
            } else if (elements.createModal.style.display !== 'none') {
                closeCreateModal();
            } else if (elements.deleteModal.style.display !== 'none') {
                closeDeleteModal();
            } else {
                elements.searchInput.value = '';
                state.searchQuery = '';
                filterAndRenderServices();
                elements.searchInput.blur();
            }
        }
    });
}

// ═══════════════════════════════════════════════════════════
// Utilities
// ═══════════════════════════════════════════════════════════

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// ═══════════════════════════════════════════════════════════
// Initialization
// ═══════════════════════════════════════════════════════════

async function init() {
    setupEventListeners();
    await fetchPlatform();
    await fetchServices();

    // Auto-refresh services every 10 seconds
    setInterval(async () => {
        await fetchServices();
        // Update selected service if still selected
        if (state.selectedService) {
            const updated = state.services.find(
                s => s.name === state.selectedService.name && s.scope === state.selectedService.scope
            );
            if (updated) {
                updateControlButtons(updated);
                elements.detailStatus.className = `status-indicator ${updated.status}`;
                state.selectedService = updated;
            }
        }
    }, 10000);
}

// Start the app
init();

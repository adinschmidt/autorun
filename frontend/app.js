// ═══════════════════════════════════════════════════════════
// autorun - Service Manager UI
// ═══════════════════════════════════════════════════════════

const state = {
    services: [],
    filteredServices: [],
    selectedService: null,
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
        elements.platformBadge.textContent = data.platform.toUpperCase();
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
        <span class="warning-text">Limited mode - run with sudo to modify system services</span>
    `;
    document.querySelector('.sidebar-header').appendChild(warning);
}

async function fetchServices() {
    try {
        const scopeParam = state.currentScope === 'all' ? '' : `?scope=${state.currentScope}`;
        state.services = await api('GET', `/api/services${scopeParam}`);
        filterAndRenderServices();
    } catch (err) {
        console.error('Failed to fetch services:', err);
        showToast('Failed to load services. Is the server running and accessible?', 'error');
        elements.serviceList.innerHTML = `
            <div class="loading-state">
                <span style="color: var(--status-stopped);">ERROR LOADING SERVICES</span>
            </div>
        `;
    }
}

async function performAction(action) {
    if (!state.selectedService) return;

    const { name, scope } = state.selectedService;

    try {
        setControlsLoading(true);
        await api('POST', `/api/services/${encodeURIComponent(name)}/${action}?scope=${scope}`);
        showToast(`${name}: ${action} successful`, 'success');

        // Wait briefly for launchd/systemd to update status before refreshing
        await new Promise(resolve => setTimeout(resolve, 500));

        // Refresh services after action
        await fetchServices();

        // Update selected service with new data
        const updated = state.services.find(s => s.name === name && s.scope === scope);
        if (updated) {
            // Update UI without reconnecting log stream
            state.selectedService = updated;
            elements.detailStatus.className = `status-indicator ${updated.status}`;
            updateControlButtons(updated);

            // Update the list item status
            const listItem = elements.serviceList.querySelector(
                `.service-item[data-name="${CSS.escape(name)}"][data-scope="${scope}"]`
            );
            if (listItem) {
                const statusDot = listItem.querySelector('.service-status');
                if (statusDot) {
                    statusDot.className = `service-status ${updated.status}`;
                }
            }
        }
    } catch (err) {
        console.error(`Action ${action} failed:`, err);
        showToast(`${action} failed: ${err.message}`, 'error');
    } finally {
        setControlsLoading(false);
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
        showToast(`Service ${name} created successfully`, 'success');
        closeCreateModal();
        await fetchServices();

        // Select the newly created service
        const newService = state.services.find(s => s.name === name && s.scope === scope);
        if (newService) {
            selectService(newService);
        }
    } catch (err) {
        console.error('Failed to create service:', err);
        showToast(`Failed to create service: ${err.message}`, 'error');
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

function openDeleteModal() {
    if (!state.selectedService) return;
    elements.deleteServiceName.textContent = state.selectedService.name;
    elements.deleteModal.style.display = 'flex';
}

function closeDeleteModal() {
    elements.deleteModal.style.display = 'none';
}

async function handleDeleteService() {
    if (!state.selectedService) return;

    const { name, scope } = state.selectedService;

    try {
        await api('DELETE', `/api/services/${encodeURIComponent(name)}?scope=${scope}`);
        showToast(`Service ${name} deleted successfully`, 'success');
        closeDeleteModal();

        // Clear selection and refresh
        state.selectedService = null;
        elements.serviceDetail.style.display = 'none';
        elements.emptyState.style.display = 'flex';

        // Close log stream
        if (state.logSocket) {
            state.logSocket.close();
            state.logSocket = null;
        }

        await fetchServices();
    } catch (err) {
        console.error('Failed to delete service:', err);
        showToast(`Failed to delete service: ${err.message}`, 'error');
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

    elements.logContent.innerHTML = '<div class="log-placeholder">Connecting to log stream...</div>';
    elements.logStatus.classList.remove('connected');
    elements.logStatus.innerHTML = '<span class="log-dot"></span>CONNECTING';

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/api/services/${encodeURIComponent(serviceName)}/logs?scope=${scope}`;

    const ws = new WebSocket(wsUrl);
    state.logSocket = ws;

    ws.onopen = () => {
        elements.logStatus.classList.add('connected');
        elements.logStatus.innerHTML = '<span class="log-dot"></span>STREAMING';
        elements.logContent.innerHTML = '';
    };

    ws.onmessage = (event) => {
        appendLogLine(event.data);
    };

    ws.onerror = (err) => {
        console.error('WebSocket error:', err);
        elements.logStatus.classList.remove('connected');
        elements.logStatus.innerHTML = '<span class="log-dot"></span>ERROR';
    };

    ws.onclose = () => {
        elements.logStatus.classList.remove('connected');
        elements.logStatus.innerHTML = '<span class="log-dot"></span>DISCONNECTED';
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
            <div class="loading-state">
                <span>NO SERVICES FOUND</span>
            </div>
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
                    <div class="service-scope">${service.scope.toUpperCase()}</div>
                </div>
                <div class="service-enabled ${service.enabled ? 'enabled' : ''}">
                    ${service.enabled ? 'ON' : 'OFF'}
                </div>
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
    elements.detailDescription.textContent = service.description || 'No description available';
    elements.detailStatus.className = `status-indicator ${service.status}`;
    elements.detailScope.textContent = service.scope.toUpperCase();

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
    elements.deleteBtn.addEventListener('click', openDeleteModal);
    elements.deleteCancel.addEventListener('click', closeDeleteModal);
    elements.deleteConfirm.addEventListener('click', handleDeleteService);
    elements.deleteModal.addEventListener('click', (e) => {
        if (e.target === elements.deleteModal) closeDeleteModal();
    });

    // Keyboard shortcuts
    document.addEventListener('keydown', (e) => {
        // Focus search on Cmd/Ctrl + K
        if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
            e.preventDefault();
            elements.searchInput.focus();
        }

        // Escape to close modals or clear search
        if (e.key === 'Escape') {
            if (elements.createModal.style.display !== 'none') {
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

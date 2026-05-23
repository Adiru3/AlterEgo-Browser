const API_URL = '/api/profiles';

let profiles = [];
let editMode = false;

// DOM Elements
const profilesList = document.getElementById('profiles-list');
const emptyState = document.getElementById('empty-state');
const profileCount = document.getElementById('profile-count');
const btnNewProfile = document.getElementById('btn-new-profile');
const modal = document.getElementById('edit-modal');
const form = document.getElementById('profile-form');
const proxyType = document.getElementById('proxy-type');
const proxyFields = document.getElementById('proxy-fields');
const btnCloseModal = document.getElementById('btn-close-modal');
const btnCancelEdit = document.getElementById('btn-cancel-edit');
const btnSaveProfile = document.getElementById('btn-save-profile');
const modalTitle = document.getElementById('modal-title');

// Init
async function init() {
    await loadProfiles();
    setupEventListeners();
}

// API Calls
async function loadProfiles() {
    try {
        const res = await fetch(API_URL);
        profiles = await res.json() || [];
        renderProfiles();
    } catch (e) {
        console.error("Failed to load profiles:", e);
    }
}

async function createProfile(name) {
    const res = await fetch(API_URL, {
        method: 'POST',
        body: JSON.stringify({ name }),
        headers: { 'Content-Type': 'application/json' }
    });
    const newProfile = await res.json();
    profiles.push(newProfile);
    renderProfiles();
    return newProfile;
}

async function updateProfile(id, data) {
    const res = await fetch(`${API_URL}/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
        headers: { 'Content-Type': 'application/json' }
    });
    const updated = await res.json();
    const idx = profiles.findIndex(p => p.id === id);
    if (idx !== -1) profiles[idx] = updated;
    renderProfiles();
}

async function deleteProfile(id) {
    if (!confirm('Are you sure you want to delete this profile?')) return;
    await fetch(`${API_URL}/${id}`, { method: 'DELETE' });
    profiles = profiles.filter(p => p.id !== id);
    renderProfiles();
}

async function launchProfile(id) {
    // Show launching state on button
    const btn = document.querySelector(`button[data-launch="${id}"]`);
    const origText = btn.innerHTML;
    btn.innerHTML = '🚀 Launching...';
    btn.disabled = true;

    try {
        await fetch(`${API_URL}/launch/${id}`, { method: 'POST' });
        // The browser window will appear
        setTimeout(() => {
            btn.innerHTML = origText;
            btn.disabled = false;
        }, 1500);
    } catch (e) {
        btn.innerHTML = 'Error!';
        setTimeout(() => {
            btn.innerHTML = origText;
            btn.disabled = false;
        }, 2000);
    }
}

// UI Rendering
function renderProfiles() {
    profileCount.innerText = profiles.length;
    
    if (profiles.length === 0) {
        profilesList.style.display = 'none';
        emptyState.style.display = 'block';
        return;
    }

    profilesList.style.display = 'grid';
    emptyState.style.display = 'none';
    profilesList.innerHTML = '';

    profiles.forEach(p => {
        const proxyText = p.proxy && p.proxy.type ? `${p.proxy.type.toUpperCase()}: ${p.proxy.host}:${p.proxy.port}` : 'Direct Connection';
        
        const card = document.createElement('div');
        card.className = 'profile-card';
        card.innerHTML = `
            <div class="card-header">
                <div class="card-title-group">
                    <div class="status-dot"></div>
                    <div>
                        <div class="profile-name">${escapeHTML(p.name)}</div>
                        <div class="profile-meta">${p.fingerprint.tls_preset || 'Chrome'} · ${p.fingerprint.screen_width}x${p.fingerprint.screen_height}</div>
                    </div>
                </div>
                <div class="card-actions">
                    <button class="btn-icon" title="Edit" onclick="openEditModal('${p.id}')">
                        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 20h9M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z"/></svg>
                    </button>
                    <button class="btn-icon danger" title="Delete" onclick="deleteProfile('${p.id}')">
                        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"></polyline><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path></svg>
                    </button>
                </div>
            </div>
            <div>
                <span class="proxy-badge">🔌 ${escapeHTML(proxyText)}</span>
            </div>
            <button class="btn btn-primary launch-btn" data-launch="${p.id}" onclick="launchProfile('${p.id}')">
                Open Browser
            </button>
        `;
        profilesList.appendChild(card);
    });
}

// Modal Logic
function openNewModal() {
    editMode = false;
    modalTitle.innerText = 'New Profile';
    form.reset();
    document.getElementById('profile-id').value = '';
    toggleProxyFields();
    modal.classList.add('active');
}

function openEditModal(id) {
    editMode = true;
    modalTitle.innerText = 'Edit Profile';
    const p = profiles.find(x => x.id === id);
    if (!p) return;

    document.getElementById('profile-id').value = p.id;
    document.getElementById('profile-name').value = p.name;
    document.getElementById('proxy-type').value = p.proxy.type || '';
    document.getElementById('proxy-host').value = p.proxy.host || '';
    document.getElementById('proxy-port').value = p.proxy.port || '';
    document.getElementById('proxy-user').value = p.proxy.user || '';
    document.getElementById('proxy-pass').value = p.proxy.pass || '';
    
    toggleProxyFields();
    modal.classList.add('active');
}

function closeModal() {
    modal.classList.remove('active');
}

function toggleProxyFields() {
    if (proxyType.value === '') {
        proxyFields.style.opacity = '0.3';
        proxyFields.style.pointerEvents = 'none';
    } else {
        proxyFields.style.opacity = '1';
        proxyFields.style.pointerEvents = 'auto';
    }
}

// Events
function setupEventListeners() {
    btnNewProfile.addEventListener('click', openNewModal);
    btnCloseModal.addEventListener('click', closeModal);
    btnCancelEdit.addEventListener('click', closeModal);
    proxyType.addEventListener('change', toggleProxyFields);

    btnSaveProfile.addEventListener('click', async () => {
        const id = document.getElementById('profile-id').value;
        const name = document.getElementById('profile-name').value;
        
        let pData = null;
        if (id) pData = profiles.find(x => x.id === id);
        
        const proxyData = {
            type: document.getElementById('proxy-type').value,
            host: document.getElementById('proxy-host').value,
            port: parseInt(document.getElementById('proxy-port').value) || 0,
            user: document.getElementById('proxy-user').value,
            pass: document.getElementById('proxy-pass').value,
        };

        if (!editMode) {
            // Create first
            pData = await createProfile(name);
        }
        
        // Update
        pData.name = name;
        pData.proxy = proxyData;
        await updateProfile(pData.id, pData);
        
        closeModal();
    });
}

function escapeHTML(str) {
    if (!str) return '';
    return str.replace(/[&<>'"]/g, 
        tag => ({
            '&': '&amp;',
            '<': '&lt;',
            '>': '&gt;',
            "'": '&#39;',
            '"': '&quot;'
        }[tag] || tag)
    );
}

// Start
init();

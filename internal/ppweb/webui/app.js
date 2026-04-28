const state = {
  bootstrap: null,
  overview: null,
  connections: [],
  clientsByConnection: {},
  editingId: null,
  loading: true,
  busy: false,
};

const app = document.querySelector("#app");
const notice = document.querySelector("#notice");
const refreshButton = document.querySelector("#refreshButton");
const viewerDialog = document.querySelector("#viewerDialog");
const viewerTitle = document.querySelector("#viewerTitle");
const viewerBody = document.querySelector("#viewerBody");
const viewerClose = document.querySelector("#viewerClose");

refreshButton.addEventListener("click", () => bootstrap());
viewerClose.addEventListener("click", () => viewerDialog.close());

bootstrap();

async function bootstrap() {
  state.loading = true;
  render();
  try {
    state.bootstrap = await apiFetch("/api/bootstrap");
    if (state.bootstrap.authenticated) {
      await loadDashboard();
    } else {
      state.overview = null;
      state.connections = [];
      state.clientsByConnection = {};
    }
  } catch (error) {
    showNotice(error.message, "error");
  } finally {
    state.loading = false;
    render();
  }
}

async function loadDashboard() {
  const [overviewResponse, connectionsResponse] = await Promise.all([
    apiFetch("/api/overview"),
    apiFetch("/api/connections"),
  ]);

  state.overview = overviewResponse;
  state.connections = connectionsResponse.connections || [];

  const nextClients = {};
  for (const connection of state.connections) {
    if (state.clientsByConnection[connection.id]) {
      nextClients[connection.id] = state.clientsByConnection[connection.id];
    }
  }
  state.clientsByConnection = nextClients;
}

function render() {
  renderNotice();
  if (state.loading) {
    app.innerHTML = `
      <section class="panel loading-panel">
        <div class="spinner"></div>
        <p>Loading control plane...</p>
      </section>
    `;
    return;
  }

  if (!state.bootstrap) {
    app.innerHTML = `
      <section class="panel loading-panel">
        <p>Bootstrap data is unavailable.</p>
      </section>
    `;
    return;
  }

  if (state.bootstrap.setupRequired) {
    renderSetup();
    return;
  }

  if (!state.bootstrap.authenticated) {
    renderLogin();
    return;
  }

  renderDashboard();
}

function renderNotice() {
  if (!state.notice) {
    notice.className = "notice hidden";
    notice.textContent = "";
    return;
  }

  notice.className = `notice ${state.notice.kind}`;
  notice.textContent = state.notice.message;
}

function renderSetup() {
  app.innerHTML = `
    <section class="panel auth-panel">
      <div class="panel-head">
        <div>
          <h2>Initial setup</h2>
          <p class="panel-subtitle">Create the first administrator for this server.</p>
        </div>
      </div>
      <div class="panel-body">
        <form id="setupForm">
          <label>
            App name
            <input name="appName" placeholder="PP Web" value="${escapeHTML(state.bootstrap.appName || "PP Web")}">
          </label>
          <label>
            Username
            <input name="username" placeholder="admin" required>
          </label>
          <label>
            Password
            <input name="password" type="password" minlength="8" required>
          </label>
          <div class="button-row">
            <button type="submit">Create administrator</button>
          </div>
        </form>
      </div>
    </section>
  `;

  document.querySelector("#setupForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    await runAction(async () => {
      await apiFetch("/api/setup", {
        method: "POST",
        body: JSON.stringify({
          appName: form.get("appName"),
          username: form.get("username"),
          password: form.get("password"),
        }),
      });
      showNotice("Administrator created.", "success");
      await bootstrap();
    });
  });
}

function renderLogin() {
  app.innerHTML = `
    <section class="panel auth-panel">
      <div class="panel-head">
        <div>
          <h2>Sign in</h2>
          <p class="panel-subtitle">Public IP: <code>${escapeHTML(state.bootstrap.publicIP || "unknown")}</code></p>
        </div>
      </div>
      <div class="panel-body">
        <form id="loginForm">
          <label>
            Username
            <input name="username" placeholder="admin" required>
          </label>
          <label>
            Password
            <input name="password" type="password" required>
          </label>
          <div class="button-row">
            <button type="submit">Sign in</button>
          </div>
        </form>
      </div>
    </section>
  `;

  document.querySelector("#loginForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    await runAction(async () => {
      await apiFetch("/api/login", {
        method: "POST",
        body: JSON.stringify({
          username: form.get("username"),
          password: form.get("password"),
        }),
      });
      showNotice("Signed in.", "success");
      await bootstrap();
    });
  });
}

function renderDashboard() {
  const overview = state.overview || {};
  const summary = overview.summary || {};
  const core = overview.core || {};
  const editingConnection = state.connections.find((connection) => connection.id === state.editingId) || null;
  const formDefaults = buildConnectionDraft(editingConnection);

  app.innerHTML = `
    <div class="dashboard">
      <section class="panel">
        <div class="panel-head">
          <div>
            <h2>${escapeHTML(state.bootstrap.appName || "PP Web")}</h2>
            <p class="panel-subtitle">Public IP: <code>${escapeHTML(state.bootstrap.publicIP || "unknown")}</code> · Listening on <code>${escapeHTML(state.bootstrap.listen || "unknown")}</code></p>
          </div>
          <div class="control-strip">
            <button class="ghost" id="syncButton">Sync config</button>
            <button class="ghost" id="restartButton">Restart pp-core</button>
            <button class="ghost" id="logoutButton">Logout</button>
          </div>
        </div>
        <div class="panel-body">
          <div class="stats">
            <div class="stat-card">
              <span>Total connections</span>
              <strong>${summary.connectionsTotal || 0}</strong>
            </div>
            <div class="stat-card">
              <span>Enabled listeners</span>
              <strong>${summary.connectionsActive || 0}</strong>
            </div>
            <div class="stat-card">
              <span>Reachable listeners</span>
              <strong>${summary.listenersReachable || 0}</strong>
            </div>
            <div class="stat-card">
              <span>Installed protocols</span>
              <strong>${summary.protocolsInstalled || 0}</strong>
            </div>
          </div>
        </div>
      </section>

      <div class="grid-two">
        <section class="panel">
          <div class="panel-head">
            <div>
              <h2>${editingConnection ? "Edit connection" : "New connection"}</h2>
              <p class="panel-subtitle">Recommended managed mode: loopback backend + HTTPS. pp-web will publish the site through nginx on <code>:443</code> automatically after HTTPS is enabled.</p>
            </div>
            <div class="button-row">
              ${editingConnection ? '<button class="ghost" id="cancelEditButton">Cancel edit</button>' : ""}
              <button class="ghost" id="generateSecretsButton">Generate secrets</button>
            </div>
          </div>
          <div class="panel-body">
            <form id="connectionForm">
              <input type="hidden" name="id" value="${editingConnection ? editingConnection.id : ""}">
              <div class="field-grid">
                <label>
                  Name
                  <input name="name" value="${escapeHTML(formDefaults.name)}" placeholder="Main gateway" required>
                </label>
                <label>
                  Tag
                  <input name="tag" value="${escapeHTML(formDefaults.tag)}" placeholder="main" required>
                </label>
                <label>
                  Listen
                  <input name="listen" value="${escapeHTML(formDefaults.listen)}" placeholder="127.0.0.1:8081" required>
                </label>
                <label>
                  Domain
                  <input name="domain" value="${escapeHTML(formDefaults.settings.domain)}" placeholder="vpn.example.com" required>
                </label>
              </div>

              <div class="field-grid">
                <label>
                  Fallback type
                  <select name="type">
                    ${selectOptions(connectionTypeOptions(formDefaults.settings.type), formDefaults.settings.type)}
                  </select>
                </label>
                <label>
                  gRPC path
                  <input name="grpc_path" value="${escapeHTML(formDefaults.settings.grpc_path)}" placeholder="/pp.v1.TunnelService/Connect">
                </label>
                <label>
                  Publish interval (minutes)
                  <input name="publish_interval_minutes" type="number" min="0" value="${escapeHTML(String(formDefaults.settings.publish_interval_minutes))}">
                </label>
                <label>
                  Publish batch size
                  <input name="publish_batch_size" type="number" min="0" value="${escapeHTML(String(formDefaults.settings.publish_batch_size))}">
                </label>
              </div>

              <div class="field-grid">
                <label>
                  Noise private key
                  <input name="noise_private_key" value="${escapeHTML(formDefaults.settings.noise_private_key)}" placeholder="base64url key" required>
                </label>
                <label>
                  Connection PSK
                  <input name="psk" value="${escapeHTML(formDefaults.settings.psk)}" placeholder="base64url key" required>
                </label>
              </div>

              <div class="field-grid">
                <label>
                  Keywords
                  <input name="scraper_keywords" value="${escapeHTML(formDefaults.settings.scraper_keywords.join(", "))}" placeholder="go, linux, devops">
                </label>
                <label>
                  Invite code
                  <input name="invite_code" value="${escapeHTML(formDefaults.settings.invite_code)}" placeholder="optional invite code">
                </label>
                <label>
                  Proxy address
                  <input name="proxy_address" value="${escapeHTML(formDefaults.settings.proxy_address)}" placeholder="127.0.0.1:8080 for proxy mode">
                </label>
              </div>

              <label>
                Server-side routing JSON
                <textarea name="routing_json" placeholder='{"default_policy":"proxy","rules":[{"type":"domain","value":"example.com","policy":"block"}]}'>${escapeHTML(formDefaults.routingJSON)}</textarea>
              </label>

              <label class="inline-check">
                <input name="enabled" type="checkbox" ${formDefaults.enabled ? "checked" : ""}>
                Enabled
              </label>

              <p class="hint">For multiple sites use different loopback ports, for example <code>127.0.0.1:8081</code>, <code>127.0.0.1:8082</code>, <code>127.0.0.1:8083</code>. After HTTPS is enabled, pp-web will publish them through nginx automatically.</p>

              <div class="button-row">
                <button type="submit">${editingConnection ? "Save connection" : "Create connection"}</button>
              </div>
            </form>
          </div>
        </section>

        <section class="panel config-preview">
          <div class="panel-head">
            <div>
              <h2>pp-core status</h2>
              <p class="panel-subtitle">${escapeHTML(core.binaryAvailable ? "Binary detected." : "Binary missing.")}</p>
            </div>
          </div>
          <div class="panel-body">
            <div class="stats">
              <div class="stat-card">
                <span>Binary path</span>
                <strong>${escapeHTML(core.binaryPath || "n/a")}</strong>
              </div>
              <div class="stat-card">
                <span>Config path</span>
                <strong>${escapeHTML(core.configPath || "n/a")}</strong>
              </div>
            </div>
            <p class="hint">Last sync: ${escapeHTML(formatDate(core.lastSyncAt))}</p>
            <p class="hint">Last error: ${escapeHTML(core.lastSyncError || "none")}</p>
            <pre>${escapeHTML(core.configPreview || "{}")}</pre>
          </div>
        </section>
      </div>

      <section class="panel">
        <div class="panel-head">
          <div>
            <h2>Connections</h2>
            <p class="panel-subtitle">Create a connection, enable HTTPS, then add per-client configs.</p>
          </div>
        </div>
        <div class="panel-body">
          <div class="connection-list">
            ${state.connections.length ? state.connections.map(renderConnectionCard).join("") : `
              <div class="connection-card">
                <div class="connection-body">
                  <p class="hint">No connections yet. Create one above. The recommended managed mode is a loopback backend such as <code>127.0.0.1:8081</code> and then enabling HTTPS, after which pp-web publishes it through nginx automatically.</p>
                </div>
              </div>
            `}
          </div>
        </div>
      </section>
    </div>
  `;

  bindDashboardEvents();
}

function bindDashboardEvents() {
  document.querySelector("#logoutButton").addEventListener("click", () => logout());
  document.querySelector("#syncButton").addEventListener("click", () => syncConfig());
  document.querySelector("#restartButton").addEventListener("click", () => restartCore());
  document.querySelector("#generateSecretsButton").addEventListener("click", () => generateSecrets());

  const cancelEditButton = document.querySelector("#cancelEditButton");
  if (cancelEditButton) {
    cancelEditButton.addEventListener("click", () => {
      state.editingId = null;
      render();
    });
  }

  document.querySelector("#connectionForm").addEventListener("submit", submitConnectionForm);

  for (const button of document.querySelectorAll("[data-edit-connection]")) {
    button.addEventListener("click", () => {
      state.editingId = Number(button.dataset.editConnection);
      render();
    });
  }

  for (const button of document.querySelectorAll("[data-delete-connection]")) {
    button.addEventListener("click", () => deleteConnection(Number(button.dataset.deleteConnection)));
  }

  for (const button of document.querySelectorAll("[data-load-clients]")) {
    button.addEventListener("click", () => loadClients(Number(button.dataset.loadClients)));
  }

  for (const button of document.querySelectorAll("[data-create-client]")) {
    button.addEventListener("click", () => createClient(Number(button.dataset.createClient)));
  }

  for (const button of document.querySelectorAll("[data-show-client-config]")) {
    button.addEventListener("click", () => showClientConfig(Number(button.dataset.connectionId), Number(button.dataset.clientId)));
  }

  for (const button of document.querySelectorAll("[data-delete-client]")) {
    button.addEventListener("click", () => deleteClient(Number(button.dataset.deleteClient)));
  }

  for (const button of document.querySelectorAll("[data-https-mode]")) {
    button.addEventListener("click", () => setupHTTPS(Number(button.dataset.connectionId), button.dataset.httpsMode));
  }
}

function renderConnectionCard(connection) {
  const settings = connection.settings || {};
  const clients = state.clientsByConnection[connection.id] || [];
  const tlsEnabled = connection.tls && connection.tls.enabled;
  const warning = runtimeWarningForConnection(connection);
  return `
    <article class="connection-card">
      <div class="connection-top">
        <div>
          <h3>${escapeHTML(connection.name)}</h3>
          <p class="panel-subtitle">${escapeHTML(settings.domain || "no domain")} · <code>${escapeHTML(connection.listen)}</code></p>
          <div class="connection-meta">
            <span class="pill ${connection.enabled ? "ok" : "bad"}">${connection.enabled ? "enabled" : "disabled"}</span>
            <span class="pill">${escapeHTML(connection.protocol)}</span>
            <span class="pill ${tlsEnabled ? "ok" : "warn"}">${tlsEnabled ? "https ready" : "tls not set"}</span>
            <span class="pill">${escapeHTML(settings.type || "blog")}</span>
          </div>
        </div>
        <div class="connection-actions">
          <button class="subtle" data-edit-connection="${connection.id}">Edit</button>
          <button class="subtle" data-load-clients="${connection.id}">Clients</button>
          <button class="ghost" data-create-client="${connection.id}">New client</button>
          <button class="ghost" data-https-mode="lets-encrypt" data-connection-id="${connection.id}">Let's Encrypt</button>
          <button class="danger" data-delete-connection="${connection.id}">Delete</button>
        </div>
      </div>
      <div class="connection-body">
        <p class="hint">Tag: <code>${escapeHTML(connection.tag)}</code></p>
        ${warning ? `<p class="hint">${escapeHTML(warning)}</p>` : ""}
        ${clients.length ? `
          <section class="clients">
            ${clients.map((client) => `
              <div class="client-row">
                <div>
                  <strong>${escapeHTML(client.name)}</strong>
                  <p class="hint">Client #${client.id} · created ${escapeHTML(formatDate(client.createdAt))}</p>
                </div>
                <div class="client-actions">
                  <button class="ghost" data-show-client-config="1" data-connection-id="${connection.id}" data-client-id="${client.id}">Show config</button>
                  <button class="danger" data-delete-client="${client.id}">Delete</button>
                </div>
              </div>
            `).join("")}
          </section>
        ` : ""}
      </div>
    </article>
  `;
}

async function submitConnectionForm(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const routingText = form.elements.routing_json.value.trim();

  let routing = null;
  if (routingText) {
    try {
      routing = JSON.parse(routingText);
    } catch (error) {
      showNotice(`Routing JSON is invalid: ${error.message}`, "error");
      return;
    }
  }

  const payload = {
    name: form.elements.name.value.trim(),
    tag: form.elements.tag.value.trim(),
    protocol: "pp-fallback",
    listen: form.elements.listen.value.trim(),
    enabled: form.elements.enabled.checked,
    tls: null,
    settings: {
      type: form.elements.type.value,
      domain: form.elements.domain.value.trim(),
      grpc_path: form.elements.grpc_path.value.trim(),
      noise_private_key: form.elements.noise_private_key.value.trim(),
      psk: form.elements.psk.value.trim(),
      proxy_address: form.elements.proxy_address.value.trim(),
      scraper_keywords: splitList(form.elements.scraper_keywords.value),
      publish_interval_minutes: parseNumber(form.elements.publish_interval_minutes.value, 60),
      publish_batch_size: parseNumber(form.elements.publish_batch_size.value, 3),
      invite_code: form.elements.invite_code.value.trim(),
      routing,
    },
  };

  const connectionId = form.elements.id.value;
  if (connectionId) {
    const existing = state.connections.find((connection) => connection.id === Number(connectionId));
    if (existing && existing.tls) {
      payload.tls = existing.tls;
    }
  }
  const method = connectionId ? "PUT" : "POST";
  const url = connectionId ? `/api/connections/${connectionId}` : "/api/connections";

  await runAction(async () => {
    const response = await apiFetch(url, {
      method,
      body: JSON.stringify(payload),
    });
    const warning = response.warning ? ` ${response.warning}` : "";
    showNotice(`Connection saved.${warning}`.trim(), response.warning ? "error" : "success");
    state.editingId = null;
    await bootstrap();
  });
}

async function generateSecrets() {
  await runAction(async () => {
    const response = await apiFetch("/api/tools/generate-secrets", {
      method: "POST",
      body: JSON.stringify({ protocol: "pp-fallback" }),
    });
    const secrets = response.secrets || {};
    const form = document.querySelector("#connectionForm");
    form.elements.noise_private_key.value = secrets.noise_private_key || "";
    form.elements.psk.value = secrets.psk || "";
    showNotice("Fresh keys generated.", "success");
  });
}

async function syncConfig() {
  await runAction(async () => {
    const response = await apiFetch("/api/pp-core/sync", { method: "POST" });
    showNotice(response.warning ? response.warning : "Config synced.", response.warning ? "error" : "success");
    await bootstrap();
  });
}

async function restartCore() {
  await runAction(async () => {
    const response = await apiFetch("/api/pp-core/restart", { method: "POST" });
    const suffix = response.method ? ` (${response.method})` : "";
    showNotice(`pp-core restarted${suffix}.`, "success");
    await bootstrap();
  });
}

async function logout() {
  await runAction(async () => {
    await apiFetch("/api/logout", { method: "POST" });
    showNotice("Logged out.", "success");
    await bootstrap();
  });
}

async function deleteConnection(connectionId) {
  if (!window.confirm("Delete this connection?")) {
    return;
  }

  await runAction(async () => {
    const response = await apiFetch(`/api/connections/${connectionId}`, { method: "DELETE" });
    showNotice(response.warning ? response.warning : "Connection deleted.", response.warning ? "error" : "success");
    if (state.editingId === connectionId) {
      state.editingId = null;
    }
    await bootstrap();
  });
}

async function loadClients(connectionId) {
  await runAction(async () => {
    const response = await apiFetch(`/api/connections/${connectionId}/clients`);
    state.clientsByConnection[connectionId] = response.clients || [];
    render();
  });
}

async function createClient(connectionId) {
  const name = window.prompt("Client name");
  if (!name) {
    return;
  }

  await runAction(async () => {
    const response = await apiFetch(`/api/connections/${connectionId}/clients`, {
      method: "POST",
      body: JSON.stringify({ name }),
    });
    const warning = response.warning ? ` ${response.warning}` : "";
    showNotice(`Client created.${warning}`.trim(), response.warning ? "error" : "success");
    await loadClients(connectionId);
    await bootstrap();
  });
}

async function showClientConfig(connectionId, clientId) {
  await runAction(async () => {
    const response = await apiFetch(`/api/connections/${connectionId}/clients/${clientId}/config`);
    const content = [
      `URI`,
      response.uri || "n/a",
      ``,
      `JSON`,
      JSON.stringify(response.config || {}, null, 2),
    ].join("\n");
    showViewer(`Client ${clientId} config`, content);
  });
}

async function deleteClient(clientId) {
  if (!window.confirm("Delete this client?")) {
    return;
  }

  await runAction(async () => {
    const response = await apiFetch(`/api/clients/${clientId}`, { method: "DELETE" });
    showNotice(response.warning ? response.warning : "Client deleted.", response.warning ? "error" : "success");
    await bootstrap();
  });
}

async function setupHTTPS(connectionId, mode) {
  if (mode !== "lets-encrypt") {
    showNotice("Only Let's Encrypt certificates are supported now.", "error");
    return;
  }

  const message = "Issue a Let's Encrypt certificate now? DNS for the domain must already point to this server.";

  if (!window.confirm(message)) {
    return;
  }

  await runAction(async () => {
    const response = await apiFetch(`/api/connections/${connectionId}/setup-https`, {
      method: "POST",
      body: JSON.stringify({ mode }),
    });
    showNotice(response.warning ? response.warning : "HTTPS updated.", response.warning ? "error" : "success");
    await bootstrap();
  });
}

async function runAction(action) {
  if (state.busy) {
    return;
  }
  state.busy = true;
  toggleBusy(true);
  try {
    await action();
  } catch (error) {
    showNotice(error.message, "error");
  } finally {
    state.busy = false;
    toggleBusy(false);
  }
}

function toggleBusy(flag) {
  for (const button of document.querySelectorAll("button")) {
    button.disabled = flag;
  }
}

function showNotice(message, kind = "success") {
  state.notice = { message, kind };
  renderNotice();
}

function showViewer(title, content) {
  viewerTitle.textContent = title;
  viewerBody.textContent = content;
  viewerDialog.showModal();
}

async function apiFetch(url, options = {}) {
  const response = await fetch(url, {
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      ...(options.headers || {}),
    },
    ...options,
  });

  const contentType = response.headers.get("content-type") || "";
  const payload = contentType.includes("application/json")
    ? await response.json()
    : { error: await response.text() };

  if (!response.ok) {
    throw new Error(payload.error || `Request failed with status ${response.status}`);
  }

  return payload;
}

function buildConnectionDraft(connection) {
  const suggestedIdentity = suggestConnectionIdentity(state.connections);
  const suggestedListen = suggestListenAddress(state.connections);
  const defaults = {
    name: suggestedIdentity.name,
    tag: suggestedIdentity.tag,
    listen: suggestedListen,
    enabled: true,
    settings: {
      type: "blog",
      domain: "",
      grpc_path: "/pp.v1.TunnelService/Connect",
      noise_private_key: "",
      psk: "",
      proxy_address: "",
      scraper_keywords: ["go", "linux", "devops"],
      publish_interval_minutes: 60,
      publish_batch_size: 3,
      invite_code: "",
    },
    routingJSON: "",
  };

  if (!connection) {
    return defaults;
  }

  const settings = connection.settings || {};
  return {
    name: connection.name || defaults.name,
    tag: connection.tag || defaults.tag,
    listen: connection.listen || defaults.listen,
    enabled: connection.enabled !== false,
    settings: {
      type: settings.type || defaults.settings.type,
      domain: settings.domain || "",
      grpc_path: settings.grpc_path || defaults.settings.grpc_path,
      noise_private_key: settings.noise_private_key || "",
      psk: settings.psk || "",
      proxy_address: settings.proxy_address || "",
      scraper_keywords: Array.isArray(settings.scraper_keywords) ? settings.scraper_keywords : defaults.settings.scraper_keywords,
      publish_interval_minutes: settings.publish_interval_minutes ?? defaults.settings.publish_interval_minutes,
      publish_batch_size: settings.publish_batch_size ?? defaults.settings.publish_batch_size,
      invite_code: settings.invite_code || "",
    },
    routingJSON: settings.routing ? JSON.stringify(settings.routing, null, 2) : "",
  };
}

function connectionTypeOptions(currentValue) {
  const options = [
    { value: "blog", label: "blog" },
    { value: "proxy", label: "proxy" },
  ];

  if (currentValue === "forum") {
    return [{ value: "forum", label: "forum (legacy)", disabled: true }, ...options];
  }

  return options;
}

function selectOptions(values, currentValue) {
  return values.map((entry) => {
    const value = typeof entry === "string" ? entry : entry.value;
    const label = typeof entry === "string" ? entry : entry.label;
    const disabled = typeof entry === "string" || !entry.disabled ? "" : " disabled";
    return `<option value="${escapeHTML(value)}" ${value === currentValue ? "selected" : ""}${disabled}>${escapeHTML(label)}</option>`;
  }).join("");
}

function splitList(value) {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function parseNumber(value, fallback) {
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function formatDate(value) {
  if (!value) {
    return "never";
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return String(value);
  }
  return date.toLocaleString();
}

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function runtimeWarningForConnection(connection) {
  const hasTLS = connection.tls && connection.tls.enabled;
  if (!hasTLS) {
    return "HTTPS is not configured yet. Clients are generated for domain:443, so enable HTTPS first and pp-web will publish this site through nginx automatically.";
  }
  return "";
}

function suggestListenAddress(connections) {
  const usedPorts = new Set();
  for (const connection of connections || []) {
    const port = parseListenPort(connection.listen);
    if (port > 0) {
      usedPorts.add(port);
    }
  }

  for (let port = 8081; port <= 8999; port += 1) {
    if (!usedPorts.has(port)) {
      return `127.0.0.1:${port}`;
    }
  }
  return "127.0.0.1:8081";
}

function suggestConnectionIdentity(connections) {
  const usedTags = new Set((connections || []).map((connection) => connection.tag));
  if (!usedTags.has("main")) {
    return { name: "Main gateway", tag: "main" };
  }

  for (let index = 2; index <= 999; index += 1) {
    const tag = `site-${index}`;
    if (!usedTags.has(tag)) {
      return { name: `Site ${index}`, tag };
    }
  }

  return { name: "Additional site", tag: `site-${Date.now()}` };
}

function parseListenPort(listen) {
  if (!listen) {
    return 0;
  }
  const match = String(listen).match(/:(\d+)$/);
  return match ? Number.parseInt(match[1], 10) : 0;
}

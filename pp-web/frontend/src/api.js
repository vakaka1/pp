const JSON_HEADERS = {
  "Content-Type": "application/json"
};

async function request(path, options = {}) {
  const response = await fetch(path, {
    credentials: "include",
    headers: {
      ...JSON_HEADERS,
      ...(options.headers ?? {})
    },
    ...options
  });

  const contentType = response.headers.get("content-type") ?? "";
  const payload = contentType.includes("application/json")
    ? await response.json()
    : { error: await response.text() };

  if (!response.ok) {
    throw new Error(payload.error || "Request failed");
  }

  return payload;
}

export const api = {
  getSettings() {
    return request("/api/settings", { method: "GET" });
  },
  saveSettings(payload) {
    return request("/api/settings", {
      method: "POST",
      body: JSON.stringify(payload)
    });
  },
  bootstrap() {
    return request("/api/bootstrap", { method: "GET" });
  },
  setup(payload) {
    return request("/api/setup", {
      method: "POST",
      body: JSON.stringify(payload)
    });
  },
  login(payload) {
    return request("/api/login", {
      method: "POST",
      body: JSON.stringify(payload)
    });
  },
  logout() {
    return request("/api/logout", {
      method: "POST",
      body: JSON.stringify({})
    });
  },
  overview() {
    return request("/api/overview", { method: "GET" });
  },
  about(refresh = false) {
    return request(`/api/about${refresh ? "?refresh=1" : ""}`, { method: "GET" });
  },
  startAboutUpdate() {
    return request("/api/about/update", {
      method: "POST",
      body: JSON.stringify({})
    });
  },
  listConnections() {
    return request("/api/connections", { method: "GET" });
  },
  saveConnection(id, payload) {
    return request(id ? `/api/connections/${id}` : "/api/connections", {
      method: id ? "PUT" : "POST",
      body: JSON.stringify(payload)
    });
  },
  deleteConnection(id) {
    return request(`/api/connections/${id}`, {
      method: "DELETE"
    });
  },
  generateSecrets(protocol) {
    return request("/api/tools/generate-secrets", {
      method: "POST",
      body: JSON.stringify({ protocol })
    });
  },
  checkPort(port) {
    return request(`/api/tools/check-port?port=${port}`, {
      method: "GET"
    });
  },
  syncCore() {
    return request("/api/pp-core/sync", {
      method: "POST",
      body: JSON.stringify({})
    });
  },
  restartCore() {
    return request("/api/pp-core/restart", {
      method: "POST",
      body: JSON.stringify({})
    });
  },
  clientConfig(id) {
    return request(`/api/connections/${id}/client-config`, { method: "GET" });
  },
  setupHTTPS(id, mode) {
    return request(`/api/connections/${id}/setup-https`, {
      method: "POST",
      body: JSON.stringify({ mode })
    });
  },
  getNginxConfig(id) {
    return request(`/api/connections/${id}/nginx-config`, { method: "GET" });
  },
  listClients(connectionId) {
    return request(`/api/connections/${connectionId}/clients`, { method: "GET" });
  },
  createClient(connectionId, name) {
    return request(`/api/connections/${connectionId}/clients`, {
      method: "POST",
      body: JSON.stringify({ name })
    });
  },
  clientConfigById(connectionId, clientId) {
    return request(`/api/connections/${connectionId}/clients/${clientId}/config`, { method: "GET" });
  },
  deleteClient(clientId) {
    return request(`/api/clients/${clientId}`, {
      method: "DELETE"
    });
  }
};

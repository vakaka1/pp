const JSON_HEADERS = {
  "Content-Type": "application/json"
};

const ROUTE_PREFIX_MARKERS = ["/app", "/login", "/setup"];

export function getPanelBasePath() {
  if (typeof window === "undefined") return "";

  const pathname = window.location.pathname || "/";
  for (const marker of ROUTE_PREFIX_MARKERS) {
    if (pathname === marker || pathname.startsWith(`${marker}/`)) {
      return "";
    }

    const index = pathname.lastIndexOf(`${marker}/`);
    if (index > 0) {
      return pathname.slice(0, index).replace(/\/+$/, "");
    }

    if (pathname.endsWith(marker)) {
      return pathname.slice(0, -marker.length).replace(/\/+$/, "");
    }
  }

  if (pathname === "/") return "";
  return pathname.replace(/\/+$/, "");
}

export function stripPanelBasePath(pathname) {
  const basePath = getPanelBasePath();
  const path = pathname || "/";
  if (!basePath) return path;
  if (path === basePath) return "/";
  if (path.startsWith(`${basePath}/`)) {
    return path.slice(basePath.length) || "/";
  }
  return path;
}

export function withPanelBasePath(path) {
  const basePath = getPanelBasePath();
  const normalizedPath = path.startsWith("/") ? path : `/${path}`;
  return `${basePath}${normalizedPath}` || "/";
}

async function request(path, options = {}) {
  const response = await fetch(withPanelBasePath(path), {
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
  rollback() {
    return request("/api/about/rollback", {
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
  restartPanel() {
    return request("/api/restart", {
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

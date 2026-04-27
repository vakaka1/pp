import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { api } from "./api";
import AboutPage from "./AboutPage";

const DEFAULT_LISTEN = "127.0.0.1:8081";
const THEME_STORAGE_KEY = "pp-web-theme";

const NAV_ITEMS = [
  {
    path: "/app/overview",
    label: "Обзор",
    shortLabel: "Обзор"
  },
  {
    path: "/app/connections",
    label: "Подключения",
    shortLabel: "Подкл."
  },
  {
    path: "/app/pp-settings",
    label: "Ядро PP",
    shortLabel: "Ядро"
  },
  {
    path: "/app/settings",
    label: "Настройки",
    shortLabel: "Настр."
  },
  {
    path: "/app/about",
    label: "О программе",
    shortLabel: "О PP"
  }
];

const THEME_OPTIONS = [
  {
    id: "light",
    label: "Светлая",
    shortLabel: "Светлая"
  },
  {
    id: "dark",
    label: "Темная",
    shortLabel: "Темная"
  }
];

const RULE_TYPES = ["geosite", "geoip", "domain", "ip_cidr", "regexp"];
const RULE_POLICIES = ["proxy", "direct", "block"];
const TYPE_LABELS = {
  geosite: "Сайты (geosite)",
  geoip: "IP страны (geoip)",
  domain: "Домен",
  ip_cidr: "IP/CIDR",
  regexp: "Regexp"
};
const POLICY_LABELS = {
  proxy: "прокси",
  direct: "напрямую",
  block: "блокировать"
};

function readInitialTheme() {
  if (typeof window === "undefined") return "light";

  try {
    const storedTheme = window.localStorage.getItem(THEME_STORAGE_KEY);
    if (storedTheme === "light" || storedTheme === "dark") {
      return storedTheme;
    }
  } catch {
    return document.documentElement.dataset.theme === "dark" ? "dark" : "light";
  }

  return document.documentElement.dataset.theme === "dark" ? "dark" : "light";
}

function applyTheme(theme) {
  document.documentElement.dataset.theme = theme;
  document.documentElement.style.colorScheme = theme;
}

function getRouteMeta(route) {
  return NAV_ITEMS.find((item) => route.startsWith(item.path)) ?? NAV_ITEMS[0];
}

function formatDateTime(isoString) {
  if (!isoString) return "—";
  const date = new Date(isoString);
  if (Number.isNaN(date.getTime())) return "—";

  return new Intl.DateTimeFormat("ru-RU", {
    dateStyle: "medium",
    timeStyle: "short"
  }).format(date);
}

function formatBuildDate(buildDate) {
  if (!buildDate) return "—";
  return buildDate.split("T")[0];
}

function getPanelHost(bootstrap) {
  if (bootstrap?.publicIP && bootstrap.publicIP !== "Unknown") {
    return bootstrap.publicIP;
  }

  return "127.0.0.1";
}

function createStatusTone(good) {
  return good ? "good" : "bad";
}

function getUpdateIndicator(aboutData, aboutError) {
  if (aboutError) {
    return {
      tone: "warning",
      label: "?"
    };
  }

  const release = aboutData?.release;
  if (release?.error && !release?.latestVersion) {
    return {
      tone: "warning",
      label: "?"
    };
  }

  if (!release?.updateAvailable) {
    return null;
  }

  if (release.indicatorTone === "danger") {
    return {
      tone: "danger",
      label: "!"
    };
  }

  return {
    tone: "warning",
    label: "!"
  };
}

function getSidebarUpdateCard(aboutData, aboutError) {
  if (aboutError) {
    return {
      tone: "warning",
      eyebrow: "Статус",
      title: "Не удалось проверить релиз",
      copy: "Откройте страницу «О программе», чтобы повторить проверку GitHub Releases.",
      action: "Проверить снова"
    };
  }

  if (!aboutData) {
    return {
      tone: "neutral",
      eyebrow: "Система",
      title: "Проверяем версию",
      copy: "Информация о сборке, GitHub Releases и состоянии обновлений загружается в фоне.",
      action: "Открыть страницу"
    };
  }

  const release = aboutData?.release;
  if (release?.error && !release?.updateAvailable) {
    return {
      tone: "warning",
      eyebrow: "Статус",
      title: "Проверка релиза недоступна",
      copy: "На странице «О программе» можно повторить запрос к GitHub Releases.",
      action: "Проверить снова"
    };
  }

  if (release?.updateAvailable) {
    const majorUpdate = release.indicatorTone === "danger";
    return {
      tone: majorUpdate ? "danger" : "warning",
      eyebrow: majorUpdate ? "Крупное обновление" : "Обновление",
      title: `Доступна версия ${release.latestVersion}`,
      copy: release.statusLabel || "Откройте «О программе», чтобы посмотреть описание релиза и обновить панель.",
      action: "Открыть релиз"
    };
  }

  return {
    tone: "neutral",
    eyebrow: "Система",
    title: "Версия актуальна",
    copy: "На странице «О программе» можно посмотреть сведения о сборке, GitHub и историю обновлений.",
    action: "Открыть страницу"
  };
}

async function copyToClipboard(value) {
  if (!value) return false;

  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value);
      return true;
    }
  } catch { }

  try {
    const textarea = document.createElement("textarea");
    textarea.value = value;
    textarea.setAttribute("readonly", "");
    textarea.style.position = "fixed";
    textarea.style.left = "-9999px";
    document.body.appendChild(textarea);
    textarea.select();
    const copied = document.execCommand("copy");
    document.body.removeChild(textarea);
    return copied;
  } catch {
    return false;
  }
}

export default function App() {
  const [bootstrap, setBootstrap] = useState(null);
  const [bootstrapError, setBootstrapError] = useState(null);
  const [route, setRoute] = useState(window.location.pathname || "/");
  const [loading, setLoading] = useState(true);
  const [notice, setNotice] = useState(null);
  const [theme, setTheme] = useState(readInitialTheme);
  const [aboutData, setAboutData] = useState(null);
  const [aboutLoading, setAboutLoading] = useState(false);
  const [aboutError, setAboutError] = useState(null);

  useEffect(() => {
    loadBootstrap();
  }, []);

  useEffect(() => {
    const handlePopState = () => setRoute(window.location.pathname || "/");
    window.addEventListener("popstate", handlePopState);
    return () => window.removeEventListener("popstate", handlePopState);
  }, []);

  useEffect(() => {
    applyTheme(theme);

    try {
      window.localStorage.setItem(THEME_STORAGE_KEY, theme);
    } catch { }
  }, [theme]);

  useEffect(() => {
    if (!bootstrap) return;

    if (bootstrap.setupRequired && route !== "/setup") {
      navigate("/setup", true);
      return;
    }

    if (!bootstrap.setupRequired && !bootstrap.authenticated && route !== "/login") {
      navigate("/login", true);
      return;
    }

    if (
      !bootstrap.setupRequired &&
      bootstrap.authenticated &&
      (route === "/" || route === "/login" || route === "/setup")
    ) {
      navigate("/app/overview", true);
    }
  }, [bootstrap, route]);

  useEffect(() => {
    if (!notice) return undefined;
    const timer = window.setTimeout(() => setNotice(null), 5000);
    return () => window.clearTimeout(timer);
  }, [notice]);

  useEffect(() => {
    if (!bootstrap || bootstrap.setupRequired || !bootstrap.authenticated) {
      setAboutData(null);
      setAboutError(null);
      setAboutLoading(false);
      return;
    }

    loadAbout({ silent: true });

    // Автоматическая фоновая проверка обновлений раз в 10 минут
    const backgroundCheckTimer = window.setInterval(() => {
      loadAbout({ silent: true });
    }, 600000);

    return () => window.clearInterval(backgroundCheckTimer);
  }, [bootstrap?.authenticated, bootstrap?.setupRequired]);

  useEffect(() => {
    const updateState = aboutData?.update?.status?.state;
    
    if (updateState === "success") {
      // Если обновление завершено успешно, проверяем версию.
      // Если версия уже совпадает с целевой, значит перезагрузка больше не нужна.
      const current = aboutData?.release?.currentVersion;
      const target = aboutData?.update?.status?.targetVersion;
      
      if (current && target) {
        const normalize = (v) => v.replace(/^v/, "");
        if (normalize(current) === normalize(target)) {
          return undefined;
        }
      }

      // Если мы всё ещё на старой версии, перезагружаем страницу
      const reloadTimer = window.setTimeout(() => {
        window.location.reload();
      }, 2500);
      return () => window.clearTimeout(reloadTimer);
    }

    if (updateState !== "queued" && updateState !== "running") {
      return undefined;
    }

    const timer = window.setInterval(() => {
      loadAbout({ force: true, silent: true });
    }, 8000);

    return () => window.clearInterval(timer);
  }, [aboutData?.update?.status?.state, aboutData?.release?.currentVersion]);

  async function loadBootstrap() {
    setLoading(true);
    setBootstrapError(null);

    try {
      const payload = await api.bootstrap();
      setBootstrap(payload);
      setBootstrapError(null);
      setRoute(window.location.pathname || "/");
    } catch (error) {
      setBootstrap(null);
      setBootstrapError(error.message);
      setNotice({ tone: "error", message: error.message });
    } finally {
      setLoading(false);
    }
  }

  async function loadAbout({ force = false, silent = false } = {}) {
    setAboutLoading(true);

    try {
      const payload = await api.about(force);
      setAboutData(payload);
      setAboutError(null);
    } catch (error) {
      setAboutError(error.message);
      if (!silent) {
        setNotice({ tone: "error", message: error.message });
      }
    } finally {
      setAboutLoading(false);
    }
  }

  function navigate(path, replace = false) {
    if (replace) {
      window.history.replaceState({}, "", path);
    } else {
      window.history.pushState({}, "", path);
    }

    setRoute(path);
  }

  async function handleLogout() {
    try {
      await api.logout();
      await loadBootstrap();
      navigate("/login", true);
    } catch (error) {
      setNotice({ tone: "error", message: error.message });
    }
  }

  if (loading || (!bootstrap && !bootstrapError)) {
    return <SplashScreen error={null} theme={theme} onThemeChange={setTheme} />;
  }

  if (!bootstrap && bootstrapError) {
    return (
      <>
        <SplashScreen
          error={bootstrapError}
          theme={theme}
          onThemeChange={setTheme}
          onRetry={loadBootstrap}
        />
        {notice &&
          createPortal(
            <div className="toast-container">
              <Banner notice={notice} onClose={() => setNotice(null)} />
            </div>,
            document.body
          )}
      </>
    );
  }

  let content;

  if (bootstrap.setupRequired) {
    content = (
      <SetupPage
        appName={bootstrap.appName}
        theme={theme}
        onThemeChange={setTheme}
        onSetup={async (payload) => {
          await api.setup(payload);
          await loadBootstrap();
          navigate("/app/overview", true);
        }}
      />
    );
  } else if (!bootstrap.authenticated) {
    content = (
      <LoginPage
        appName={bootstrap.appName}
        theme={theme}
        onThemeChange={setTheme}
        onLogin={async (payload) => {
          await api.login(payload);
          await loadBootstrap();
          navigate("/app/overview", true);
        }}
      />
    );
  } else {
    content = (
      <Shell
        bootstrap={bootstrap}
        route={route}
        user={bootstrap.user}
        build={bootstrap.build}
        aboutData={aboutData}
        aboutLoading={aboutLoading}
        aboutError={aboutError}
        theme={theme}
        onThemeChange={setTheme}
        onNavigate={navigate}
        onLogout={handleLogout}
        onRefreshAbout={loadAbout}
        onNotice={setNotice}
      />
    );
  }

  return (
    <>
      {content}
      {notice &&
        createPortal(
          <div className="toast-container">
            <Banner notice={notice} onClose={() => setNotice(null)} />
          </div>,
          document.body
        )}
    </>
  );
}

function SplashScreen({ error, theme, onThemeChange, onRetry }) {
  return (
    <OnboardingLayout appName="PP Web" theme={theme} onThemeChange={onThemeChange}>
      <div className="auth-card auth-card--center">
        <div className="status-stack">
          <div className="status-orb" />
          <div>
            <h2>{error ? "Ошибка загрузки" : "Загрузка"}</h2>
            {error ? <p>{error}</p> : null}
          </div>
        </div>

        {error ? (
          <button className="primary-button" onClick={onRetry}>
            Повторить загрузку
          </button>
        ) : (
          <div className="loading-track" aria-hidden="true">
            <span />
          </div>
        )}
      </div>
    </OnboardingLayout>
  );
}

function SetupPage({ appName, theme, onThemeChange, onSetup }) {
  const [form, setForm] = useState({
    appName: appName || "PP Web",
    username: "",
    password: ""
  });
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event) {
    event.preventDefault();
    setSubmitting(true);

    try {
      await onSetup(form);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <OnboardingLayout appName={appName || "PP Web"} theme={theme} onThemeChange={onThemeChange}>
      <div className="auth-card">
        <div className="auth-card__head">
          <h2>Настройка системы</h2>
        </div>

        <form onSubmit={handleSubmit} className="auth-form">

          <div className="input-group">
            <label>Имя администратора</label>
            <input
              type="text"
              required
              value={form.username}
              onChange={(event) => setForm({ ...form, username: event.target.value })}
              placeholder="admin"
            />
          </div>

          <div className="input-group">
            <label>Пароль</label>
            <input
              type="password"
              required
              minLength={8}
              value={form.password}
              onChange={(event) => setForm({ ...form, password: event.target.value })}
              placeholder="Минимум 8 символов"
            />
          </div>

          <button type="submit" className="primary-button primary-button--wide" disabled={submitting}>
            {submitting ? "Сохраняем конфигурацию..." : "Завершить настройку"}
          </button>
        </form>
      </div>
    </OnboardingLayout>
  );
}

function LoginPage({ appName, theme, onThemeChange, onLogin }) {
  const [form, setForm] = useState({ username: "", password: "" });
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event) {
    event.preventDefault();
    setSubmitting(true);

    try {
      await onLogin(form);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <OnboardingLayout appName={appName} theme={theme} onThemeChange={onThemeChange}>
      <div className="auth-card">
        <div className="auth-card__head">
          <h2>Вход в систему</h2>
        </div>

        <form onSubmit={handleSubmit} className="auth-form">
          <div className="input-group">
            <label>Имя пользователя</label>
            <input
              type="text"
              required
              value={form.username}
              onChange={(event) => setForm({ ...form, username: event.target.value })}
              placeholder="admin"
            />
          </div>

          <div className="input-group">
            <label>Пароль</label>
            <input
              type="password"
              required
              value={form.password}
              onChange={(event) => setForm({ ...form, password: event.target.value })}
              placeholder="Ваш пароль"
            />
          </div>

          <button type="submit" className="primary-button primary-button--wide" disabled={submitting}>
            {submitting ? "Проверяем доступ..." : "Войти"}
          </button>
        </form>
      </div>
    </OnboardingLayout>
  );
}

function OnboardingLayout({
  appName,
  theme,
  onThemeChange,
  children
}) {
  return (
    <div className="welcome-shell">
      <div className="welcome-shell__aurora welcome-shell__aurora--one" />
      <div className="welcome-shell__aurora welcome-shell__aurora--two" />

      <div className="welcome-shell__inner">
        <header className="welcome-topbar">
          <BrandLockup appName={appName || "PP Web"} />
          <ThemeSwitcher value={theme} onChange={onThemeChange} />
        </header>

        <div className="welcome-grid">
          <section className="welcome-form-column">{children}</section>
        </div>
      </div>
    </div>
  );
}

function ThemeSwitcher({ value, onChange, compact = false }) {
  return (
    <div className={`theme-switcher ${compact ? "theme-switcher--compact" : ""}`}>
      {THEME_OPTIONS.map((option) => (
        <button
          key={option.id}
          type="button"
          className={`theme-switcher__button ${value === option.id ? "is-active" : ""}`}
          onClick={() => onChange?.(option.id)}
        >
          <span className={`theme-switcher__swatch theme-switcher__swatch--${option.id}`} />
          <span>{compact ? option.shortLabel : option.label}</span>
        </button>
      ))}
    </div>
  );
}

function BrandLockup({ appName, subtitle, compact = false }) {
  return (
    <div className={`brand-lockup ${compact ? "brand-lockup--compact" : ""}`}>
      <div className="brand-lockup__copy">
        <strong>{appName}</strong>
        {subtitle ? <span>{subtitle}</span> : null}
      </div>
    </div>
  );
}

function Shell({
  bootstrap,
  route,
  user,
  build,
  aboutData,
  aboutLoading,
  aboutError,
  theme,
  onThemeChange,
  onNavigate,
  onLogout,
  onRefreshAbout,
  onNotice
}) {
  const routeMeta = getRouteMeta(route);
  const updateIndicator = getUpdateIndicator(aboutData, aboutError);
  const sidebarUpdateCard = getSidebarUpdateCard(aboutData, aboutError);

  let content = null;
  if (route.startsWith("/app/overview")) {
    content = <OverviewPage onNotice={onNotice} />;
  } else if (route.startsWith("/app/connections")) {
    content = <ConnectionsPage onNotice={onNotice} />;
  } else if (route.startsWith("/app/pp-settings")) {
    content = <PPSettingsPage onNotice={onNotice} />;
  } else if (route.startsWith("/app/settings")) {
    content = (
      <SettingsPage
        user={user}
        build={build}
        bootstrap={bootstrap}
        theme={theme}
        onThemeChange={onThemeChange}
        onNotice={onNotice}
      />
    );
  } else if (route.startsWith("/app/about")) {
    content = (
      <AboutPage
        data={aboutData}
        loading={aboutLoading}
        error={aboutError}
        onRefresh={onRefreshAbout}
        onNotice={onNotice}
      />
    );
  }

  return (
    <div className="app-shell">
      <div className="app-shell__backdrop" />

      <aside className="app-sidebar">
        <div className="app-sidebar__inner">
          <div className="app-sidebar__brand-card">
            <BrandLockup appName={bootstrap.appName} />
          </div>

          <nav className="sidebar-nav">
            {NAV_ITEMS.map((item) => (
              <NavItem
                key={item.path}
                label={item.label}
                active={route.startsWith(item.path)}
                indicator={item.path === "/app/about" ? updateIndicator : null}
                onClick={() => onNavigate(item.path)}
              />
            ))}
          </nav>

          <button
            className={`sidebar-update-card sidebar-update-card--${sidebarUpdateCard.tone}`}
            onClick={() => onNavigate("/app/about")}
          >
            <div className="sidebar-update-card__eyebrow">{sidebarUpdateCard.eyebrow}</div>
            <div className="sidebar-update-card__copy">
              <h3>{sidebarUpdateCard.title}</h3>
              <p>{sidebarUpdateCard.copy}</p>
            </div>
            <span className="sidebar-update-card__action">{sidebarUpdateCard.action}</span>
          </button>
        </div>
      </aside>

      <div className="app-main">
        <header className="app-topbar">
          <div className="app-topbar__primary">
            <div className="app-topbar__mobile-brand">
              <BrandLockup appName={bootstrap.appName} compact />
            </div>
            <div className="app-topbar__copy">
              <span className="eyebrow">PP Web</span>
              <h1>{routeMeta.label}</h1>
            </div>
          </div>

          <div className="app-topbar__actions">
            <div className="topbar-theme">
              <ThemeSwitcher value={theme} onChange={onThemeChange} compact />
            </div>
            <button className="ghost-button ghost-button--quiet" onClick={onLogout}>
              Выйти
            </button>
          </div>
        </header>

        <main className="app-main__body">{content}</main>
      </div>

      <nav className="mobile-dock">
        {NAV_ITEMS.map((item) => (
          <button
            key={item.path}
            className={`mobile-dock__item ${route.startsWith(item.path) ? "is-active" : ""}`}
            onClick={() => onNavigate(item.path)}
          >
            {item.path === "/app/about" && updateIndicator ? (
              <span className={`mobile-dock__indicator mobile-dock__indicator--${updateIndicator.tone}`}>
                {updateIndicator.label}
              </span>
            ) : null}
            <span>{item.shortLabel || item.label}</span>
          </button>
        ))}
      </nav>
    </div>
  );
}

function NavItem({ label, active, indicator, onClick }) {
  return (
    <button className={`nav-item ${active ? "active" : ""}`} onClick={onClick}>
      <span className="nav-item__label">{label}</span>
      {indicator ? (
        <span className={`nav-item__indicator nav-item__indicator--${indicator.tone}`}>{indicator.label}</span>
      ) : null}
    </button>
  );
}

function PageHero({ eyebrow, title, description, actions, aside, tone = "default" }) {
  return (
    <section className={`page-hero page-hero--${tone}`}>
      <div className="page-hero__copy">
        <span className="eyebrow">{eyebrow}</span>
        <h2>{title}</h2>
        {description ? <p>{description}</p> : null}
        {actions ? <div className="page-hero__actions">{actions}</div> : null}
      </div>

      {aside ? <div className="page-hero__aside">{aside}</div> : null}
    </section>
  );
}

function MetricCard({ value, label }) {
  return (
    <article className="metric-card">
      <div className="metric-value">{value}</div>
      <div className="metric-label">{label}</div>
    </article>
  );
}

function StatusPill({ tone, good, children }) {
  const resolvedTone = tone || createStatusTone(good);
  return <span className={`status-pill status-pill--${resolvedTone}`}>{children}</span>;
}

function Detail({ label, value }) {
  return (
    <div className="detail-row">
      <dt>{label}</dt>
      <dd>{value || "—"}</dd>
    </div>
  );
}

function Banner({ notice, onClose }) {
  if (!notice) return null;

  return (
    <div className={`banner banner--${notice.tone}`}>
      <span className="banner-message">{notice.message}</span>
      {onClose ? (
        <button className="banner-close" onClick={onClose}>
          ×
        </button>
      ) : null}
    </div>
  );
}

function PageSkeleton({ title, description }) {
  return (
    <div className="page">
      <section className="page-hero page-hero--loading">
        <div className="page-hero__copy">
          <span className="eyebrow skeleton-text">Загрузка</span>
          <h2 className="skeleton-text">{title}</h2>
          {description ? <p className="skeleton-text">{description}</p> : null}
        </div>
      </section>

      <div className="metric-grid">
        <div className="skeleton-box" style={{ height: "154px" }} />
        <div className="skeleton-box" style={{ height: "154px" }} />
        <div className="skeleton-box" style={{ height: "154px" }} />
        <div className="skeleton-box" style={{ height: "154px" }} />
      </div>

      <div className="dashboard-grid">
        <div className="skeleton-box" style={{ height: "320px" }} />
        <div className="skeleton-box" style={{ height: "320px" }} />
      </div>
    </div>
  );
}

function OverviewPage({ onNotice }) {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadOverview();
  }, []);

  async function loadOverview() {
    setLoading(true);

    try {
      const payload = await api.overview();
      setData(payload);
    } catch (error) {
      onNotice({ tone: "error", message: error.message });
    } finally {
      setLoading(false);
    }
  }

  if (loading || !data) {
    return <PageSkeleton title="Обзор системы" />;
  }

  const runningListeners = data.listeners.filter((listener) => listener.enabled && listener.reachable);
  const healthyCore = data.core.binaryAvailable && data.core.configValid;

  return (
    <div className="page">

      <section className="metric-grid">
        <MetricCard value={data.summary.connectionsTotal} label="Всего профилей" />
        <MetricCard value={data.summary.connectionsActive} label="Активных профилей" />
        <MetricCard value={data.summary.listenersReachable} label="Доступных слушателей" />
        <MetricCard value={data.summary.protocolsInstalled} label="Протоколов" />
      </section>
    </div>
  );
}

function ConnectionsPage({ onNotice }) {
  const [connections, setConnections] = useState([]);
  const [protocols, setProtocols] = useState([]);
  const [loading, setLoading] = useState(true);
  const [editorOpen, setEditorOpen] = useState(false);
  const [editingConnection, setEditingConnection] = useState(null);
  const [clientsOpen, setClientsOpen] = useState(null);
  const [httpsChoice, setHttpsChoice] = useState(null);
  const [nginxConfigOpen, setNginxConfigOpen] = useState(null);

  useEffect(() => {
    loadConnections();
  }, []);

  async function loadConnections() {
    setLoading(true);

    try {
      const payload = await api.listConnections();
      setConnections(payload.connections || []);
      setProtocols(payload.protocols || []);
    } catch (error) {
      onNotice({ tone: "error", message: error.message });
    } finally {
      setLoading(false);
    }
  }

  async function handleDelete(connection) {
    if (!window.confirm(`Удалить подключение «${connection.name}»?`)) return;

    try {
      const payload = await api.deleteConnection(connection.id);
      await loadConnections();
      onNotice({
        tone: payload.warning ? "warning" : "success",
        message: payload.warning || `Подключение «${connection.name}» удалено.`
      });
    } catch (error) {
      onNotice({ tone: "error", message: error.message });
    }
  }

  return (
    <div className="page">
      <div className="page-toolbar">
        <button
          className="primary-button"
          onClick={() => {
            setEditingConnection(null);
            setEditorOpen(true);
          }}
          disabled={loading}
        >
          Создать подключение
        </button>
      </div>

      {loading ? (
        <PageSkeleton title="Подключения" />
      ) : (
        <section className="connections-grid">
          {connections.length ? (
            connections.map((connection) => (
              <article className="connection-card" key={connection.id}>
                <div className="connection-head">
                  <div>
                    <span className="eyebrow">{connection.protocol}</span>
                    <h3>{connection.name}</h3>
                  </div>
                  <StatusPill good={connection.enabled}>
                    {connection.enabled ? "Активен" : "Отключен"}
                  </StatusPill>
                </div>

                <div className="connection-body">
                  <div className="connection-facts">
                    <div className="connection-fact">
                      <span>Порт</span>
                      <strong>{connection.listen}</strong>
                    </div>
                    <div className="connection-fact">
                      <span>Домен</span>
                      <strong>{connection.settings?.domain || "—"}</strong>
                    </div>
                    <div className="connection-fact">
                      <span>Тип</span>
                      <strong>
                        {connection.settings?.type === "forum"
                          ? "Форум"
                          : connection.settings?.type === "blog"
                            ? "Блог"
                            : "—"}
                      </strong>
                    </div>
                    <div className="connection-fact">
                      <span>Tag</span>
                      <strong>{connection.tag || "—"}</strong>
                    </div>
                  </div>

                  <div className="connection-badges">
                    <StatusPill tone={connection.tls?.enabled ? "good" : "neutral"}>
                      {connection.tls?.enabled ? "HTTPS включен" : "Без HTTPS"}
                    </StatusPill>
                    <StatusPill tone="neutral">
                      {connection.settings?.routing?.rules?.length || 0} правил роутинга
                    </StatusPill>
                  </div>
                </div>

                <div className="connection-actions">
                  <div className="button-group">
                    <button className="ghost-button" onClick={() => setClientsOpen(connection)}>
                      Клиенты
                    </button>
                    <button
                      className="ghost-button"
                      onClick={() => {
                        setEditingConnection(connection);
                        setEditorOpen(true);
                      }}
                    >
                      Настроить
                    </button>
                    <button className="ghost-button" onClick={() => setNginxConfigOpen(connection)}>
                      Nginx
                    </button>
                  </div>

                  <button className="ghost-button destructive" onClick={() => handleDelete(connection)}>
                    Удалить
                  </button>
                </div>
              </article>
            ))
          ) : (
            <div className="empty-state">
              <span className="eyebrow">Пусто</span>
              <h3>Подключений пока нет</h3>
            </div>
          )}
        </section>
      )}

      {editorOpen ? (
        <ConnectionEditor
          connection={editingConnection}
          protocols={protocols}
          onClose={() => setEditorOpen(false)}
          onSaved={async (id, payload) => {
            setEditorOpen(false);

            try {
              const result = await api.saveConnection(id, payload);
              await loadConnections();
              onNotice({
                tone: result.warning ? "warning" : "success",
                message: result.warning || "Подключение сохранено."
              });

              if (!id) {
                setHttpsChoice(result.connection);
              }
            } catch (error) {
              onNotice({ tone: "error", message: error.message });
            }
          }}
          onNotice={onNotice}
        />
      ) : null}

      {clientsOpen ? (
        <ClientsModal
          connection={clientsOpen}
          onClose={() => setClientsOpen(null)}
          onNotice={onNotice}
        />
      ) : null}

      {httpsChoice ? (
        <HTTPSChoiceModal
          connection={httpsChoice}
          onClose={() => setHttpsChoice(null)}
          onNotice={onNotice}
          onUpdated={loadConnections}
          onShowNginx={(connection) => setNginxConfigOpen(connection)}
        />
      ) : null}

      {nginxConfigOpen ? (
        <NginxModal
          connection={nginxConfigOpen}
          onClose={() => setNginxConfigOpen(null)}
          onNotice={onNotice}
        />
      ) : null}
    </div>
  );
}

function HTTPSChoiceModal({ connection, onClose, onNotice, onUpdated, onShowNginx }) {
  const [busy, setBusy] = useState(false);

  async function handleApply(type) {
    setBusy(true);

    try {
      await api.setupHTTPS(connection.id, type);
      onNotice({ tone: "success", message: `HTTPS (${type}) успешно настроен.` });
      await onUpdated();
      onClose();
      onShowNginx(connection);
    } catch (error) {
      onNotice({ tone: "error", message: error.message });
    } finally {
      setBusy(false);
    }
  }

  return createPortal(
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal-window" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <span className="eyebrow">HTTPS</span>
            <h3>Настройка сертификата</h3>
          </div>
          <button className="icon-button" onClick={onClose}>
            ×
          </button>
        </div>

        <div className="modal-body">
          <p className="modal-intro">
            Профиль создан. Можно сразу включить HTTPS для домена <strong>{connection.settings.domain}</strong>.
          </p>

          <div className="choice-grid">
            <button className="choice-card" onClick={() => handleApply("self-signed")} disabled={busy}>
              <h4>Самоподписанный</h4>
              <p>Быстрый тестовый сценарий. Браузер покажет предупреждение, но запуск будет мгновенным.</p>
            </button>
            <button className="choice-card" onClick={() => handleApply("lets-encrypt")} disabled={busy}>
              <h4>Let's Encrypt</h4>
              <p>Бесплатный боевой сертификат. Для проверки нужен доступный 80 порт.</p>
            </button>
          </div>
        </div>

        <div className="modal-footer">
          <button className="ghost-button" onClick={onClose} disabled={busy}>
            Пропустить сейчас
          </button>
        </div>
      </div>
    </div>,
    document.body
  );
}

function NginxModal({ connection, onClose, onNotice }) {
  const [config, setConfig] = useState("");
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadConfig();
  }, []);

  async function loadConfig() {
    setLoading(true);

    try {
      const result = await api.getNginxConfig(connection.id);
      setConfig(result.config);
    } catch (error) {
      onNotice({ tone: "error", message: error.message });
    } finally {
      setLoading(false);
    }
  }

  async function handleCopy() {
    const copied = await copyToClipboard(config);
    onNotice({
      tone: copied ? "success" : "error",
      message: copied ? "Конфиг Nginx скопирован." : "Не удалось скопировать конфиг."
    });
  }

  return createPortal(
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal-window modal-window--large" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <span className="eyebrow">Nginx</span>
            <h3>{connection.settings.domain}</h3>
          </div>
          <button className="icon-button" onClick={onClose}>
            ×
          </button>
        </div>

        <div className="modal-body scrollable">
          <p className="modal-intro">
            Скопируйте конфиг и положите его, например, в <code>/etc/nginx/sites-available/{connection.settings.domain}</code>.
          </p>

          {loading ? (
            <div className="loader">Генерация...</div>
          ) : (
            <>
              <div className="modal-actions-top">
                <button className="ghost-button" onClick={handleCopy}>
                  Копировать конфиг
                </button>
              </div>
              <pre className="json-panel json-panel--modal">{config}</pre>
            </>
          )}
        </div>

        <div className="modal-footer">
          <button className="primary-button" onClick={onClose}>
            Готово
          </button>
        </div>
      </div>
    </div>,
    document.body
  );
}

function ClientsModal({ connection, onClose, onNotice }) {
  const [clients, setClients] = useState([]);
  const [loading, setLoading] = useState(true);
  const [newName, setNewName] = useState("");
  const [adding, setAdding] = useState(false);
  const [clientPreview, setClientPreview] = useState(null);
  const [previewLoading, setPreviewLoading] = useState(false);

  useEffect(() => {
    loadClients();
  }, []);

  async function loadClients() {
    setLoading(true);

    try {
      const result = await api.listClients(connection.id);
      setClients(result.clients || []);
    } catch (error) {
      onNotice({ tone: "error", message: error.message });
    } finally {
      setLoading(false);
    }
  }

  async function handleAdd(event) {
    event.preventDefault();
    if (!newName.trim()) return;

    setAdding(true);

    try {
      await api.createClient(connection.id, newName.trim());
      const clientName = newName.trim();
      setNewName("");
      await loadClients();
      onNotice({ tone: "success", message: `Клиент «${clientName}» создан.` });
    } catch (error) {
      onNotice({ tone: "error", message: error.message });
    } finally {
      setAdding(false);
    }
  }

  async function handleDelete(client) {
    if (!window.confirm(`Удалить клиента «${client.name}»?`)) return;

    try {
      await api.deleteClient(client.id);
      await loadClients();
      onNotice({ tone: "success", message: `Клиент «${client.name}» удалён.` });
    } catch (error) {
      onNotice({ tone: "error", message: error.message });
    }
  }

  async function fetchClientData(client) {
    setPreviewLoading(true);

    try {
      const payload = await api.clientConfigById(connection.id, client.id);
      return {
        name: client.name,
        uri: payload.uri || null,
        configJson: JSON.stringify(payload.config, null, 2)
      };
    } finally {
      setPreviewLoading(false);
    }
  }

  async function handleView(client) {
    try {
      const data = await fetchClientData(client);
      setClientPreview(data);
    } catch (error) {
      onNotice({ tone: "error", message: error.message });
    }
  }

  async function handleDownload(client) {
    try {
      const data = await fetchClientData(client);
      const blob = new Blob([data.configJson], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = `${client.name.replace(/[^a-zA-Z0-9_-]/g, "_")}.json`;
      anchor.click();
      URL.revokeObjectURL(url);
    } catch (error) {
      onNotice({ tone: "error", message: error.message });
    }
  }

  return createPortal(
    <div className="modal-backdrop" onClick={onClose}>
      <div
        className="modal-window modal-window--large modal-window--scroll"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="modal-header">
          <div>
            <span className="eyebrow">Клиенты</span>
            <h3>{connection.name}</h3>
            <p className="modal-subtitle">{connection.settings?.domain || "Без домена"}</p>
          </div>
          <button className="icon-button" onClick={onClose}>
            ×
          </button>
        </div>

        <div className="modal-body scrollable">
          <form onSubmit={handleAdd} className="add-client-form">
            <input
              type="text"
              placeholder="Имя клиента: PC, Home, Работа..."
              value={newName}
              onChange={(event) => setNewName(event.target.value)}
              required
              maxLength={64}
            />
            <button className="primary-button" type="submit" disabled={adding}>
              {adding ? "Создание..." : "Добавить"}
            </button>
          </form>

          {loading ? (
            <div className="loader">Загрузка...</div>
          ) : clients.length ? (
            <div className="client-list">
              {clients.map((client) => (
                <div className="client-row" key={client.id}>
                  <div className="client-info">
                    <strong>{client.name}</strong>
                    <p className="muted-caption">Создан: {formatDateTime(client.createdAt)}</p>
                  </div>

                  <div className="client-actions">
                    <button
                      className="ghost-button ghost-button--small"
                      onClick={() => handleDownload(client)}
                      disabled={previewLoading}
                      title="Скачать готовый .json конфиг"
                    >
                      Скачать
                    </button>
                    <button
                      className="ghost-button ghost-button--small"
                      onClick={() => handleView(client)}
                      disabled={previewLoading}
                    >
                      {previewLoading ? "..." : "Смотреть"}
                    </button>
                    <button
                      className="ghost-button ghost-button--small destructive"
                      onClick={() => handleDelete(client)}
                    >
                      Удалить
                    </button>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <p className="empty-muted">Клиентов пока нет. Добавьте первого выше.</p>
          )}
        </div>
      </div>

      {clientPreview ? (
        <ClientConfigModal
          preview={clientPreview}
          onClose={() => setClientPreview(null)}
          onNotice={onNotice}
        />
      ) : null}
    </div>,
    document.body
  );
}

function ClientConfigModal({ preview, onClose, onNotice }) {
  const [showUri, setShowUri] = useState(false);

  function downloadJson() {
    const blob = new Blob([preview.configJson], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `${preview.name.replace(/[^a-zA-Z0-9_-]/g, "_")}.json`;
    anchor.click();
    URL.revokeObjectURL(url);
  }

  async function handleCopyJson() {
    const copied = await copyToClipboard(preview.configJson);
    onNotice({
      tone: copied ? "success" : "error",
      message: copied ? "JSON-конфиг скопирован." : "Не удалось скопировать JSON."
    });
  }

  async function handleCopyUri() {
    const copied = await copyToClipboard(preview.uri);
    onNotice({
      tone: copied ? "success" : "error",
      message: copied ? "URI скопирован." : "Не удалось скопировать URI."
    });
  }

  return createPortal(
    <div className="modal-backdrop" onClick={onClose}>
      <div
        className="modal-window modal-window--large modal-window--scroll"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="modal-header">
          <div>
            <span className="eyebrow">Client config</span>
            <h3>{preview.name}</h3>
            <p className="modal-subtitle">Конфиг pp-fallback клиента</p>
          </div>
          <button className="icon-button" onClick={onClose}>
            ×
          </button>
        </div>

        <div className="modal-body scrollable">
          <div className="config-download-block">
            <div className="config-download-info">
              <span className="config-format-badge">JSON</span>
              <div>
                <strong>Конфигурационный файл</strong>
                <p className="muted-caption">Запуск: <code>pp client --config файл.json</code></p>
              </div>
            </div>

            <div className="button-group">
              <button className="ghost-button ghost-button--small" onClick={handleCopyJson}>
                Копировать
              </button>
              <button className="primary-button primary-button--sm" onClick={downloadJson}>
                Скачать .json
              </button>
            </div>
          </div>

          <pre className="json-panel json-panel--modal">{preview.configJson}</pre>

          {preview.uri ? (
            <>
              <button
                className="ghost-button ghost-button--small"
                style={{ marginTop: "1.25rem" }}
                onClick={() => setShowUri(!showUri)}
              >
                {showUri ? "Скрыть compact URI" : "Показать compact URI"}
              </button>

              {showUri ? (
                <div className="uri-block" style={{ marginTop: "0.75rem" }}>
                  <div className="uri-label">
                    <span>ppf:// URI</span>
                    <button className="ghost-button ghost-button--small" onClick={handleCopyUri}>
                      Копировать
                    </button>
                  </div>
                  <pre className="uri-value">{preview.uri}</pre>
                  <p className="uri-note">
                    Все параметры подключения в одной строке. Импорт URI можно будет легко передавать вручную между устройствами.
                  </p>
                </div>
              ) : null}
            </>
          ) : null}
        </div>

        <div className="modal-footer">
          <button className="ghost-button" onClick={onClose}>
            Закрыть
          </button>
          <button className="primary-button" onClick={downloadJson}>
            Скачать .json
          </button>
        </div>
      </div>
    </div>,
    document.body
  );
}

function PPSettingsPage({ onNotice }) {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [syncing, setSyncing] = useState(false);
  const [restarting, setRestarting] = useState(false);
  const [showAdvanced, setShowAdvanced] = useState(false);

  useEffect(() => {
    loadData();
  }, []);

  async function loadData() {
    setLoading(true);

    try {
      const payload = await api.overview();
      setData(payload);
    } catch (error) {
      onNotice({ tone: "error", message: error.message });
    } finally {
      setLoading(false);
    }
  }

  async function handleSync() {
    setSyncing(true);

    try {
      const payload = await api.syncCore();
      onNotice({
        tone: payload.warning ? "warning" : "success",
        message: payload.warning || "Конфигурация обновлена."
      });
      await loadData();
    } catch (error) {
      onNotice({ tone: "error", message: error.message });
    } finally {
      setSyncing(false);
    }
  }

  async function handleRestart() {
    setRestarting(true);

    try {
      await api.restartCore();
      onNotice({ tone: "success", message: "Ядро успешно перезапущено." });
      await loadData();
    } catch (error) {
      onNotice({ tone: "error", message: error.message });
    } finally {
      setRestarting(false);
    }
  }

  if (loading || !data) {
    return <PageSkeleton title="Ядро PP" />;
  }

  const isCoreReady = data.core.binaryAvailable && data.core.configValid;
  const isRunning = data.listeners.some((listener) => listener.enabled && listener.reachable);

  return (
    <div className="page">
      <PageHero
        eyebrow="Runtime"
        title="Управление ядром PP"
        actions={
          <div className="page-hero__button-row">
            <button className="ghost-button" onClick={handleSync} disabled={syncing}>
              {syncing ? "Синхронизация..." : "Обновить конфигурацию"}
            </button>
            <button
              className={`primary-button ${isRunning ? "warning" : "success"}`}
              onClick={handleRestart}
              disabled={restarting || !data.core.binaryAvailable}
            >
              {restarting ? "Подождите..." : isRunning ? "Перезапустить ядро" : "Запустить систему"}
            </button>
          </div>
        }
        aside={
          <div className="runtime-status-card">
            <div className={`runtime-status-card__indicator ${isRunning ? "is-live" : ""}`} />
            <div>
              <span>Текущий статус</span>
              <strong>{isRunning ? "Система работает" : "Система остановлена"}</strong>
            </div>
          </div>
        }
        tone="runtime"
      />

      {!data.core.binaryAvailable ? (
        <Banner
          notice={{
            tone: "error",
            message: "Исполняемый файл 'pp' не найден. Без него запуск ядра невозможен."
          }}
        />
      ) : null}

      <section className="insight-grid">
        <article className="surface-card">
          <div className="surface-card__head">
            <div>
              <span className="eyebrow">Status</span>
              <h3>Что сейчас происходит</h3>
            </div>
            <StatusPill tone={isCoreReady ? "good" : "warning"}>
              {isCoreReady ? "Готово к работе" : "Нужна проверка"}
            </StatusPill>
          </div>

          <div className="detail-grid">
            <div className="detail-card">
              <span>Бинарник</span>
              <strong>{data.core.binaryAvailable ? "OK" : "Отсутствует"}</strong>
              <p>{data.core.binaryPath || "Путь не определён"}</p>
            </div>
            <div className="detail-card">
              <span>Конфиг</span>
              <strong>{data.core.configValid ? "Валиден" : "Ошибка"}</strong>
              <p>{data.core.configPath}</p>
            </div>
            <div className="detail-card">
              <span>Последняя синхронизация</span>
              <strong>{formatDateTime(data.core.lastSyncAt)}</strong>
              <p>{data.core.lastSyncError || "Без ошибок"}</p>
            </div>
          </div>
        </article>

        <article className="surface-card">
          <div className="surface-card__head">
            <div>
              <span className="eyebrow">Ports</span>
              <h3>Сетевые слушатели</h3>
            </div>
          </div>

          <div className="mini-listener-list">
            {data.listeners.length ? (
              data.listeners.map((listener) => (
                <div key={listener.id} className="mini-listener">
                  <span>{listener.name}</span>
                  <StatusPill good={listener.reachable}>
                    {listener.reachable ? "Активен" : "Ожидание"}
                  </StatusPill>
                </div>
              ))
            ) : (
              <p className="empty-muted">Нет активных слушателей.</p>
            )}
          </div>
        </article>
      </section>

      <div className="advanced-toggle">
        <button onClick={() => setShowAdvanced(!showAdvanced)}>
          {showAdvanced ? "Скрыть технические детали" : "Показать технические детали"}
        </button>
      </div>

      {showAdvanced ? (
        <section className="panel-grid fade-in">
          <article className="surface-card">
            <div className="surface-card__head">
              <div>
                <span className="eyebrow">Build info</span>
                <h3>Пути и версии</h3>
              </div>
            </div>

            <dl className="details-list">
              <Detail label="Бинарный файл" value={data.core.binaryPath} />
              <Detail label="Файл конфигурации" value={data.core.configPath} />
              <Detail label="Версия" value={data.core.binaryVersion || "Неизвестно"} />
              <Detail label="Последняя синхронизация" value={formatDateTime(data.core.lastSyncAt)} />
            </dl>
          </article>

          <article className="surface-card">
            <div className="surface-card__head">
              <div>
                <span className="eyebrow">Preview</span>
                <h3>Текущий конфиг (JSON)</h3>
              </div>
            </div>

            <pre className="json-panel">{data.core.configPreview}</pre>
          </article>
        </section>
      ) : null}
    </div>
  );
}

function SettingsPage({ user, build, bootstrap, theme, onThemeChange, onNotice }) {
  const [panelDomain, setPanelDomain] = useState("");
  const [showPanelNginx, setShowPanelNginx] = useState(false);

  const publicHost = getPanelHost(bootstrap);
  const panelUrl = `http://${publicHost}:4090`;
  const panelNginxConfig = `server {
    server_name ${panelDomain || publicHost};
    listen 80;

    location / {
        proxy_pass http://127.0.0.1:4090;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}`;

  async function handleCopy(text, successMessage) {
    const copied = await copyToClipboard(text);
    onNotice({
      tone: copied ? "success" : "error",
      message: copied ? successMessage : "Не удалось скопировать значение."
    });
  }

  return (
    <div className="page">


      <section className="settings-grid">
        <article className="surface-card surface-card--wide">
          <div className="surface-card__head">
            <div>
              <span className="eyebrow">Access</span>
              <h3>Доступ к панели</h3>
            </div>
          </div>

          <div className="address-display">
            <code className="big-address">{panelUrl}</code>
            <button className="ghost-button" onClick={() => handleCopy(panelUrl, "Адрес панели скопирован.")}>
              Копировать
            </button>
          </div>

          <div className="nginx-hint-box">
            <h4>Красивый домен для панели</h4>

            <div className="input-group">
              <label>Домен панели</label>
              <input
                type="text"
                placeholder="panel.example.com"
                value={panelDomain}
                onChange={(event) => setPanelDomain(event.target.value)}
              />
            </div>

            <div className="button-group button-group--wrap">
              <button
                className="ghost-button ghost-button--small"
                onClick={() => setShowPanelNginx(!showPanelNginx)}
              >
                {showPanelNginx ? "Скрыть конфиг" : "Показать конфиг Nginx"}
              </button>
              <button
                className="ghost-button ghost-button--small"
                onClick={() => handleCopy(panelNginxConfig, "Конфиг Nginx для панели скопирован.")}
              >
                Копировать конфиг
              </button>
            </div>

            {showPanelNginx ? <pre className="json-panel json-panel--small">{panelNginxConfig}</pre> : null}
          </div>
        </article>

        <article className="surface-card">
          <div className="surface-card__head">
            <div>
              <span className="eyebrow">Profile</span>
              <h3>Администратор</h3>
            </div>
          </div>

          <dl className="details-list">
            <Detail label="Логин" value={user?.username || "—"} />
            <Detail label="Роль" value="Суперпользователь" />
            <Detail label="Публичный IP" value={bootstrap.publicIP || "Unknown"} />
            <Detail label="Backend listen" value={bootstrap.listen || DEFAULT_LISTEN} />
          </dl>
        </article>
      </section>
    </div>
  );
}

function RoutingRulesEditor({ routing, onChange }) {
  const rules = routing.rules || [];

  function setPolicy(value) {
    onChange({ ...routing, default_policy: value });
  }

  function addRule() {
    onChange({
      ...routing,
      rules: [...rules, { type: "geosite", value: "", policy: "block", comment: "" }]
    });
  }

  function updateRule(index, field, value) {
    const nextRules = rules.map((rule, ruleIndex) =>
      ruleIndex === index ? { ...rule, [field]: value } : rule
    );
    onChange({ ...routing, rules: nextRules });
  }

  function deleteRule(index) {
    onChange({ ...routing, rules: rules.filter((_, ruleIndex) => ruleIndex !== index) });
  }

  function moveRule(index, direction) {
    const nextRules = [...rules];
    const swapIndex = index + direction;
    if (swapIndex < 0 || swapIndex >= nextRules.length) return;
    [nextRules[index], nextRules[swapIndex]] = [nextRules[swapIndex], nextRules[index]];
    onChange({ ...routing, rules: nextRules });
  }

  return (
    <div className="routing-editor">
      <div className="routing-policy-row">
        <span className="routing-policy-label">Политика по умолчанию</span>
        <div className="routing-policy-btns">
          {RULE_POLICIES.map((policy) => (
            <button
              key={policy}
              type="button"
              className={`policy-pill policy-pill--${policy} ${routing.default_policy === policy ? "policy-pill--active" : ""
                }`}
              onClick={() => setPolicy(policy)}
            >
              {POLICY_LABELS[policy]}
            </button>
          ))}
        </div>
      </div>

      <div className="rule-cards">
        {rules.length === 0 ? (
          <p className="empty-muted" style={{ margin: 0, padding: "0.75rem", fontSize: "0.8125rem" }}>
            Нет правил, действует только политика по умолчанию.
          </p>
        ) : null}

        {rules.map((rule, index) => (
          <div className="rule-card" key={index}>
            <div className="rule-card-head">
              <div className="rule-card-selects">
                <select
                  className="rule-select"
                  value={rule.type}
                  onChange={(event) => updateRule(index, "type", event.target.value)}
                >
                  {RULE_TYPES.map((type) => (
                    <option key={type} value={type}>
                      {TYPE_LABELS[type] || type}
                    </option>
                  ))}
                </select>

                <span className="rule-arrow">→</span>

                <select
                  className={`rule-select rule-policy-select rule-policy-select--${rule.policy}`}
                  value={rule.policy}
                  onChange={(event) => updateRule(index, "policy", event.target.value)}
                >
                  {RULE_POLICIES.map((policy) => (
                    <option key={policy} value={policy}>
                      {POLICY_LABELS[policy] || policy}
                    </option>
                  ))}
                </select>
              </div>

              <div className="rule-card-actions">
                <button
                  type="button"
                  className="icon-button icon-button--tiny"
                  onClick={() => moveRule(index, -1)}
                  disabled={index === 0}
                  title="Выше"
                >
                  ↑
                </button>
                <button
                  type="button"
                  className="icon-button icon-button--tiny"
                  onClick={() => moveRule(index, 1)}
                  disabled={index === rules.length - 1}
                  title="Ниже"
                >
                  ↓
                </button>
                <button
                  type="button"
                  className="icon-button icon-button--del"
                  onClick={() => deleteRule(index)}
                  title="Удалить"
                >
                  ×
                </button>
              </div>
            </div>

            <input
              className="rule-value-input"
              placeholder={
                rule.type === "geosite" || rule.type === "geoip"
                  ? "Код страны или категория (ru, cn, ...)"
                  : rule.type === "ip_cidr"
                    ? "10.0.0.0/8"
                    : "example.com"
              }
              value={rule.value}
              onChange={(event) => updateRule(index, "value", event.target.value)}
            />

            <input
              className="rule-comment-input"
              placeholder="Комментарий (необязательно)"
              value={rule.comment || ""}
              onChange={(event) => updateRule(index, "comment", event.target.value)}
            />
          </div>
        ))}
      </div>

      <button type="button" className="ghost-button ghost-button--small rule-add-btn" onClick={addRule}>
        Добавить правило
      </button>
    </div>
  );
}

function ConnectionEditor({ connection, protocols, onClose, onSaved, onNotice }) {
  const defaultRouting = {
    default_policy: "proxy",
    rules: []
  };

  const [form, setForm] = useState({
    name: connection?.name || "",
    enabled: connection ? connection.enabled : true,
    protocol: connection?.protocol || protocols[0]?.id || "pp-fallback",
    port: connection?.listen?.split(":").pop() || "8081",
    tag: connection?.tag || "",
    settings: connection?.settings || {
      type: "blog",
      domain: "",
      scraper_keywords: [],
      noise_private_key: "",
      psk: ""
    }
  });
  const [routing, setRouting] = useState(connection?.settings?.routing ?? defaultRouting);
  const [showRouting, setShowRouting] = useState(false);
  const [saving, setSaving] = useState(false);
  const [portStatus, setPortStatus] = useState(null);

  async function handleCheckPort() {
    if (!form.port) return;

    setPortStatus("checking");

    try {
      const result = await api.checkPort(form.port);
      setPortStatus(result.available ? "available" : "taken");
    } catch {
      setPortStatus(null);
      onNotice({ tone: "error", message: "Ошибка при проверке порта." });
    }
  }

  async function handleSubmit(event) {
    event.preventDefault();
    setSaving(true);

    const { port, ...formData } = form;
    const payload = {
      ...formData,
      listen: `:${port}`,
      tag: formData.tag || `tag-${port}`,
      settings: { ...formData.settings, routing }
    };

    if (
      form.protocol === "pp-fallback" &&
      (!payload.settings.noise_private_key || !payload.settings.psk)
    ) {
      try {
        const result = await api.generateSecrets("pp-fallback");
        payload.settings.noise_private_key =
          payload.settings.noise_private_key || result.secrets.noise_private_key;
        payload.settings.psk = payload.settings.psk || result.secrets.psk;
      } catch {
        onNotice({ tone: "error", message: "Не удалось сгенерировать ключи." });
        setSaving(false);
        return;
      }
    }

    try {
      await onSaved(connection?.id, payload);
    } catch (error) {
      onNotice({ tone: "error", message: error.message });
      setSaving(false);
    }
  }

  return createPortal(
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal-window modal-window--form" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <span className="eyebrow">Connection editor</span>
            <h3>{connection ? "Изменить подключение" : "Создать подключение"}</h3>
          </div>
          <button className="icon-button" onClick={onClose}>
            ×
          </button>
        </div>

        <form onSubmit={handleSubmit} className="modal-form">
          <div className="modal-body scrollable">
            <div className="form-grid">
              <div className="input-group">
                <label>Имя подключения</label>
                <input
                  type="text"
                  required
                  value={form.name}
                  onChange={(event) => setForm({ ...form, name: event.target.value })}
                  placeholder="Например: блог-1 или форум"
                />
              </div>

              <div className="input-group">
                <label>Протокол</label>
                <select
                  disabled={Boolean(connection)}
                  value={form.protocol}
                  onChange={(event) => setForm({ ...form, protocol: event.target.value })}
                >
                  {protocols.map((protocol) => (
                    <option key={protocol.id} value={protocol.id}>
                      {protocol.name}
                    </option>
                  ))}
                </select>
              </div>

              <div className="input-group">
                <label>Порт подключения</label>
                <div className="input-with-action">
                  <input
                    type="number"
                    required
                    min="1"
                    max="65535"
                    value={form.port}
                    onChange={(event) => {
                      setForm({ ...form, port: event.target.value });
                      setPortStatus(null);
                    }}
                    placeholder="8081"
                  />
                  <button
                    type="button"
                    className="ghost-button ghost-button--small"
                    onClick={handleCheckPort}
                    disabled={portStatus === "checking"}
                  >
                    {portStatus === "checking" ? "..." : "Проверить"}
                  </button>
                </div>
                {portStatus === "available" ? <span className="status-hint good">Порт свободен</span> : null}
                {portStatus === "taken" ? <span className="status-hint bad">Порт занят</span> : null}
              </div>

              {form.protocol === "pp-fallback" ? (
                <>
                  <div className="input-group">
                    <label>Домен</label>
                    <input
                      type="text"
                      required
                      value={form.settings.domain}
                      onChange={(event) =>
                        setForm({
                          ...form,
                          settings: { ...form.settings, domain: event.target.value }
                        })
                      }
                      placeholder="example.com"
                    />
                  </div>

                  <div className="input-group">
                    <label>Тип сайта</label>
                    <select
                      value={form.settings.type}
                      onChange={(event) =>
                        setForm({
                          ...form,
                          settings: { ...form.settings, type: event.target.value }
                        })
                      }
                    >
                      <option value="blog">Блог</option>
                      <option value="forum">Форум</option>
                    </select>
                  </div>

                  <div className="input-group">
                    <label>Теги для парсера</label>
                    <input
                      type="text"
                      value={(form.settings.scraper_keywords || []).join(", ")}
                      onChange={(event) => {
                        const keywords = event.target.value
                          .split(",")
                          .map((keyword) => keyword.trim())
                          .filter(Boolean);
                        setForm({
                          ...form,
                          settings: { ...form.settings, scraper_keywords: keywords }
                        });
                      }}
                      placeholder="Жизнь в лесу, новые технологии"
                    />
                    <p className="muted-caption">Статьи будут подбираться по этим тегам.</p>
                  </div>
                </>
              ) : null}

              <div className="section-divider" />

              <div className="section-header-row">
                <h4>Серверный роутинг</h4>
                <button
                  type="button"
                  className="ghost-button ghost-button--small"
                  onClick={() => setShowRouting(!showRouting)}
                >
                  {showRouting ? "Скрыть" : "Настроить"}
                </button>
              </div>

              <p className="muted-caption">
                Правила применяются на сервере для всех клиентов этого подключения. Клиенты обновляются автоматически.
              </p>

              {showRouting ? <RoutingRulesEditor routing={routing} onChange={setRouting} /> : null}

              <div className="checkbox-group">
                <label>
                  <input
                    type="checkbox"
                    checked={form.enabled}
                    onChange={(event) => setForm({ ...form, enabled: event.target.checked })}
                  />
                  <span>Включить подключение сразу после сохранения</span>
                </label>
              </div>
            </div>
          </div>

          <div className="modal-footer">
            <button type="button" className="ghost-button" onClick={onClose} disabled={saving}>
              Отмена
            </button>
            <button type="submit" className="primary-button" disabled={saving}>
              {saving ? "Сохранение..." : connection ? "Сохранить изменения" : "Создать"}
            </button>
          </div>
        </form>
      </div>
    </div>,
    document.body
  );
}

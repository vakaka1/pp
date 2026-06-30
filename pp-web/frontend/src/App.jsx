import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { api, stripPanelBasePath, withPanelBasePath } from "./api";
import AboutPage from "./AboutPage";

const DEFAULT_LISTEN = "127.0.0.1:8081";
const THEME_STORAGE_KEY = "pp-web-theme";

const ICONS = {
  profiles: (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" /><circle cx="9" cy="7" r="4" /><path d="M22 21v-2a4 4 0 0 0-3-3.87" /><path d="M16 3.13a4 4 0 0 1 0 7.75" /></svg>
  ),
  activity: (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12" /></svg>
  ),
  network: (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><rect x="2" y="2" width="20" height="8" rx="2" ry="2" /><rect x="2" y="14" width="20" height="8" rx="2" ry="2" /><line x1="6" y1="6" x2="6.01" y2="6" /><line x1="6" y1="18" x2="6.01" y2="18" /></svg>
  ),
  protocol: (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><polyline points="16 18 22 12 16 6" /><polyline points="8 6 2 12 8 18" /></svg>
  )
};

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

const RULE_TYPES = ["geosite", "geoip", "domain", "domain_suffix", "domain_keyword", "ip_cidr", "domain_regex"];
const RULE_POLICIES = ["proxy", "direct", "block"];
const TYPE_LABELS = {
  geosite: "Сайты (geosite)",
  geoip: "IP страны (geoip)",
  domain: "Домен",
  domain_suffix: "Суффикс домена",
  domain_keyword: "Ключевое слово домена",
  ip_cidr: "IP/CIDR",
  domain_regex: "Regexp"
};
const POLICY_LABELS = {
  proxy: "разрешить",
  direct: "напрямую",
  block: "запретить"
};

const POLICY_DESCRIPTIONS = {
  proxy: "Клиент отправит трафик через PP, сервер откроет соединение к цели.",
  direct: "Клиент откроет соединение к цели сам, мимо PP-туннеля.",
  block: "Клиент и сервер отклонят соединение."
};

const RULE_TYPE_HELP = {
  geosite: "Категория доменов из geosite, например ru.",
  geoip: "Страна IP из GeoIP, например ru.",
  domain: "Домен и его поддомены, например example.com.",
  domain_suffix: "Суффикс домена, например .ru.",
  domain_keyword: "Любой домен с этим фрагментом.",
  ip_cidr: "Диапазон IP, например 192.168.0.0/16.",
  domain_regex: "Регулярное выражение для домена."
};

const ROUTING_PRESETS = [
  {
    label: "Разрешить домен",
    rule: { type: "domain", value: "", policy: "proxy", comment: "" }
  },
  {
    label: "Напрямую домен",
    rule: { type: "domain", value: "", policy: "direct", comment: "" }
  },
  {
    label: "Запретить домен",
    rule: { type: "domain", value: "", policy: "block", comment: "" }
  }
];

function normalizeRoutingForSave(routing) {
  const defaultPolicy = routing?.default_policy || "proxy";
  const rules = (routing?.rules || [])
    .map((rule) => ({
      ...rule,
      value: rule.value?.trim() || "",
      comment: rule.comment?.trim() || ""
    }))
    .filter((rule) => rule.type && rule.policy && rule.value);

  return { ...routing, default_policy: defaultPolicy, rules };
}

function describeRoutingRule(rule) {
  const value = rule.value?.trim();
  if (!value) return "Черновик без цели";

  switch (rule.type) {
    case "domain":
      return `${value} и поддомены`;
    case "domain_suffix":
      return `домены с суффиксом ${value}`;
    case "domain_keyword":
      return `домены с фрагментом "${value}"`;
    case "ip_cidr":
      return `IP диапазон ${value}`;
    case "geoip":
      return `IP страны ${value.toUpperCase()}`;
    case "geosite":
      return `сайты категории ${value}`;
    case "domain_regex":
      return `домены по regexp ${value}`;
    default:
      return value;
  }
}

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
  if (!isoString || isoString.startsWith("0001-01-01")) return "—";
  const date = new Date(isoString);
  if (Number.isNaN(date.getTime())) return "—";

  return new Intl.DateTimeFormat("ru-RU", {
    dateStyle: "medium",
    timeStyle: "short"
  }).format(date);
}

function formatBytes(bytes) {
  if (bytes === 0 || !bytes) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
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

function getSiteTypeLabel(type) {
  if (type === "forum") return "Форум";
  if (type === "proxy") return "Прокси";
  return "Новости";
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

function dedupeTags(values) {
  const seen = new Set();
  const result = [];

  values.forEach((value) => {
    const normalized = value.trim();
    if (!normalized) return;

    const key = normalized.toLowerCase();
    if (seen.has(key)) return;

    seen.add(key);
    result.push(normalized);
  });

  return result;
}

export default function App() {
  const [bootstrap, setBootstrap] = useState(null);
  const [bootstrapError, setBootstrapError] = useState(null);
  const [route, setRoute] = useState(() => stripPanelBasePath(window.location.pathname || "/"));
  const [loading, setLoading] = useState(true);
  const [notice, setNotice] = useState(null);
  const [theme, setTheme] = useState(readInitialTheme);
  const [aboutData, setAboutData] = useState(null);
  const [aboutLoading, setAboutLoading] = useState(false);
  const [aboutError, setAboutError] = useState(null);
  const [confirmDialog, setConfirmDialog] = useState(null);
  const [notifiedUpdateVersion, setNotifiedUpdateVersion] = useState("");

  useEffect(() => {
    loadBootstrap();
  }, []);

  useEffect(() => {
    const handlePopState = () => setRoute(stripPanelBasePath(window.location.pathname || "/"));
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

    // При загрузке страницы сразу проверяем обновления (с force, чтобы сбросить кеш)
    loadAbout({ force: true, silent: true });

    // Автоматическая фоновая проверка обновлений раз в 10 минут
    const backgroundCheckTimer = window.setInterval(() => {
      loadAbout({ silent: true });
    }, 600000);

    return () => window.clearInterval(backgroundCheckTimer);
  }, [bootstrap?.authenticated, bootstrap?.setupRequired]);

  useEffect(() => {
    const updateState = aboutData?.update?.status?.state;

    if (updateState === "success") {
      const current = aboutData?.release?.currentVersion;
      const target = aboutData?.update?.status?.targetVersion;

      if (current && target) {
        const normalize = (v) => v.replace(/^v/, "");
        if (normalize(current) === normalize(target)) {
          return undefined;
        }
      }

      // После успешного обновления ждём немного и пробуем перезагрузить страницу.
      // Если сервер временно недоступен (перезапускается), повторяем попытки.
      let attempts = 0;
      const maxAttempts = 30;

      const tryReload = () => {
        attempts++;
        if (attempts > maxAttempts) {
          window.location.reload();
          return;
        }

        api.about(true)
          .then(() => {
            window.location.reload();
          })
          .catch(() => {
            retryTimer = window.setTimeout(tryReload, 2000);
          });
      };

      let retryTimer = window.setTimeout(tryReload, 2500);
      return () => window.clearTimeout(retryTimer);
    }

    if (updateState !== "queued" && updateState !== "running") {
      return undefined;
    }

    const timer = window.setInterval(() => {
      loadAbout({ force: true, silent: true });
    }, 8000);

    return () => window.clearInterval(timer);
  }, [aboutData?.update?.status?.state, aboutData?.release?.currentVersion]);

  useEffect(() => {
    const release = aboutData?.release;
    if (!release?.updateAvailable || !release.latestVersion) return;
    if (release.latestVersion === notifiedUpdateVersion) return;

    setNotifiedUpdateVersion(release.latestVersion);
    setNotice({
      tone: release.indicatorTone === "danger" ? "warning" : "success",
      message: `Доступно обновление ${release.latestVersion}. Откройте «О программе», чтобы запустить установку вручную.`
    });
  }, [aboutData?.release?.updateAvailable, aboutData?.release?.latestVersion, notifiedUpdateVersion]);

  async function loadBootstrap() {
    setLoading(true);
    setBootstrapError(null);

    try {
      const payload = await api.bootstrap();
      setBootstrap(payload);
      setBootstrapError(null);
      setRoute(stripPanelBasePath(window.location.pathname || "/"));
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
    const browserPath = withPanelBasePath(path);
    if (replace) {
      window.history.replaceState({}, "", browserPath);
    } else {
      window.history.pushState({}, "", browserPath);
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
        onConfirm={requestConfirmation}
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
      {confirmDialog ? (
        <ConfirmDialog
          {...confirmDialog}
          onCancel={() => {
            confirmDialog.resolve(false);
            setConfirmDialog(null);
          }}
          onConfirm={() => {
            confirmDialog.resolve(true);
            setConfirmDialog(null);
          }}
        />
      ) : null}
    </>
  );

  function requestConfirmation(options) {
    return new Promise((resolve) => {
      setConfirmDialog({
        title: options?.title || "Подтвердите действие",
        message: options?.message || "",
        confirmLabel: options?.confirmLabel || "Продолжить",
        cancelLabel: options?.cancelLabel || "Отмена",
        tone: options?.tone || "warning",
        resolve
      });
    });
  }
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
  const [error, setError] = useState("");

  async function handleSubmit(event) {
    event.preventDefault();
    setSubmitting(true);
    setError("");

    try {
      await onLogin(form);
    } catch (loginError) {
      setError(loginError.message || "Неверное имя пользователя или пароль.");
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

          {error ? <div className="form-error">{error}</div> : null}
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
  onNotice,
  onConfirm
}) {
  const routeMeta = getRouteMeta(route);
  const updateIndicator = getUpdateIndicator(aboutData, aboutError);
  const sidebarUpdateCard = getSidebarUpdateCard(aboutData, aboutError);

  let content = null;
  if (route.startsWith("/app/overview")) {
    content = <OverviewPage onNotice={onNotice} />;
  } else if (route.startsWith("/app/connections")) {
    content = <ConnectionsPage onNotice={onNotice} onConfirm={onConfirm} />;
  } else if (route.startsWith("/app/pp-settings")) {
    content = <PPSettingsPage onNotice={onNotice} />;
  } else if (route.startsWith("/app/settings")) {
    content = (
      <SettingsPage
        bootstrap={bootstrap}
        onNotice={onNotice}
        onConfirm={onConfirm}
        onRefreshAbout={onRefreshAbout}
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
        onConfirm={onConfirm}
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
        <button
          className={`mobile-dock__item ${route.startsWith("/app/about") ? "is-active" : ""}`}
          onClick={() => onNavigate("/app/about")}
        >
          {updateIndicator ? (
            <span className={`mobile-dock__indicator mobile-dock__indicator--${updateIndicator.tone}`}>
              {updateIndicator.label}
            </span>
          ) : null}
          <span>О прогр.</span>
        </button>
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

function MetricCard({ value, label, icon }) {
  return (
    <div className="metric-card">
      {icon && <div className="metric-card__icon">{icon}</div>}
      <span className="metric-value">{value}</span>
      <span className="metric-label">{label}</span>
    </div>
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

function ConfirmDialog({
  title,
  message,
  confirmLabel,
  cancelLabel,
  tone,
  onConfirm,
  onCancel
}) {
  return createPortal(
    <div className="modal-backdrop confirm-backdrop" onClick={onCancel}>
      <div className="modal-window confirm-dialog" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <span className="eyebrow">Подтверждение</span>
            <h3>{title}</h3>
          </div>
          <button className="icon-button" onClick={onCancel} aria-label="Закрыть">
            ×
          </button>
        </div>
        <div className="modal-body confirm-dialog__body">
          <p>{message}</p>
        </div>
        <div className="modal-footer">
          <button type="button" className="ghost-button" onClick={onCancel}>
            {cancelLabel}
          </button>
          <button type="button" className={`primary-button ${tone === "danger" ? "danger" : "warning"}`} onClick={onConfirm}>
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>,
    document.body
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

function StatusOrb({ good, pulse = true }) {
  return <div className={`health-orb health-orb--${good ? "good" : "bad"} ${pulse ? "is-pulsing" : ""}`} />;
}

function CoreStatusCard({ core, activeConnections }) {
  const processExpected = activeConnections > 0;
  const isHealthy = core.binaryAvailable && core.configValid && (processExpected ? core.processRunning : true);

  return (
    <section className="surface-card">
      <header className="surface-card__head">
        <div className="eyebrow">Состояние системы</div>
        <StatusOrb good={isHealthy} />
      </header>

      <div className="core-status-list">
        <div className="core-status-item">
          <span>Версия ядра</span>
          <strong>{core.binaryVersion || "—"}</strong>
        </div>
        <div className="core-status-item">
          <span>Синхронизация</span>
          <strong>{formatDateTime(core.lastSyncAt)}</strong>
        </div>
        <div className="core-status-item">
          <span>Конфигурация</span>
          <strong style={{ color: core.configValid ? "var(--success)" : "var(--error)" }}>
            {core.configValid ? "Валидна" : "Ошибка"}
          </strong>
        </div>
        <div className="core-status-item">
          <span>Процесс</span>
          <strong style={{ color: core.processRunning ? "var(--success)" : (processExpected ? "var(--error)" : "var(--text-muted)") }}>
            {core.processRunning ? "Запущен" : (processExpected ? "Остановлен" : "Ожидание")}
          </strong>
        </div>
      </div>

      {core.lastSyncError && (
        <div className="core-error-box" style={{ marginTop: "1rem", padding: "0.75rem", borderRadius: "8px", background: "var(--error-bg)", color: "var(--error)", fontSize: "0.85rem" }}>
          {core.lastSyncError}
        </div>
      )}
    </section>
  );
}

function ProtocolUsageList({ protocols }) {
  const activeProtocols = protocols.filter((p) => p.usageCount > 0);

  return (
    <section className="surface-card">
      <header className="surface-card__head">
        <div className="eyebrow">Используемые протоколы</div>
      </header>

      {activeProtocols.length > 0 ? (
        <div className="protocol-usage-list">
          {activeProtocols.map((p) => (
            <div key={p.id} className="protocol-usage-item">
              <div className="protocol-usage-dot" style={{ background: p.accent || "var(--accent-strong)" }} />
              <span className="protocol-usage-name">{p.name}</span>
              <span className="protocol-usage-count">{p.usageCount}</span>
            </div>
          ))}
        </div>
      ) : (
        <div className="empty-muted">Нет активных протоколов</div>
      )}
    </section>
  );
}

function ListenersSummaryList({ listeners }) {
  const activeListeners = listeners.filter((l) => l.enabled);

  return (
    <section className="surface-card">
      <header className="surface-card__head">
        <div className="eyebrow">Доступность слушателей</div>
      </header>

      <div className="listeners-summary-list">
        {activeListeners.length > 0 ? (
          activeListeners.map((l) => (
            <div key={l.id} className="listener-summary-item">
              <div className="listener-summary-info">
                <div className={`listener-summary-status listener-summary-status--${l.reachable ? "good" : "bad"}`} />
                <div>
                  <div className="listener-summary-addr">{l.listen}</div>
                  <div className="listener-summary-proto">{l.protocol} — {l.name}</div>
                </div>
              </div>
              <StatusPill tone={l.reachable ? "good" : "bad"}>
                {l.reachable ? "Доступен" : "Офлайн"}
              </StatusPill>
            </div>
          ))
        ) : (
          <div className="empty-muted">Нет активных слушателей</div>
        )}
      </div>
    </section>
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
    <div className="page fade-in">
      <div className="dashboard-layout">

        <header className="dashboard-hero">
          <span className="eyebrow" style={{ color: "var(--accent-strong)" }}>Сводка</span>
          <p>
            Система работает в штатном режиме. У вас запущено <strong>{runningListeners.length}</strong> слушателей
            и настроено <strong>{data.summary.connectionsTotal}</strong> профилей подключений.
          </p>
        </header>

        <section className="dashboard-stats-grid">
          <MetricCard
            value={data.summary.connectionsTotal}
            label="Всего профилей"
            icon={ICONS.profiles}
          />
          <MetricCard
            value={data.summary.connectionsActive}
            label="Активных"
            icon={ICONS.activity}
          />
          <MetricCard
            value={data.summary.listenersReachable}
            label="Онлайн"
            icon={ICONS.network}
          />
          <MetricCard
            value={data.summary.protocolsInstalled}
            label="Протоколов"
            icon={ICONS.protocol}
          />
        </section>

        <div className="dashboard-insights">
          <div style={{ display: "flex", flexDirection: "column", gap: "1.5rem", width: "97%" }}>
            <CoreStatusCard core={data.core} activeConnections={data.summary.connectionsActive} />
            <ProtocolUsageList protocols={data.protocols} />
          </div>
          <ListenersSummaryList listeners={data.listeners} />
        </div>

      </div>
    </div>
  );
}

function ConnectionsPage({ onNotice, onConfirm }) {
  const [connections, setConnections] = useState([]);
  const [protocols, setProtocols] = useState([]);
  const [loading, setLoading] = useState(true);
  const [editorOpen, setEditorOpen] = useState(false);
  const [editingConnection, setEditingConnection] = useState(null);
  const [clientsOpen, setClientsOpen] = useState(null);
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
    const confirmed = await onConfirm?.({
      title: "Удалить подключение?",
      message: `Подключение «${connection.name}» будет удалено вместе с его клиентскими записями. Конфиги, которые уже выданы клиентам, могут перестать работать.`,
      confirmLabel: "Удалить",
      tone: "danger"
    });
    if (!confirmed) return;

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
                      <strong>{getSiteTypeLabel(connection.settings?.type)}</strong>
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
          connections={connections}
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

              if (!result.connection?.tls?.enabled) {
                try {
                  await api.setupHTTPS(result.connection.id, "lets-encrypt");
                  await loadConnections();
                  onNotice({ tone: "success", message: "Подключение сохранено, HTTPS настроен автоматически." });
                } catch (error) {
                  onNotice({ tone: "warning", message: `Подключение сохранено, но HTTPS не настроен: ${error.message}` });
                }
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
            onConfirm={onConfirm}
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

function ClientsModal({ connection, onClose, onNotice, onConfirm }) {
  const [clients, setClients] = useState([]);
  const [loading, setLoading] = useState(true);
  const [newName, setNewName] = useState("");
  const [adding, setAdding] = useState(false);
  const [clientPreview, setClientPreview] = useState(null);
  const [previewLoading, setPreviewLoading] = useState(false);

  useEffect(() => {
    loadClients();
    const intervalID = window.setInterval(() => {
      loadClients({ silent: true });
    }, 5000);
    return () => window.clearInterval(intervalID);
  }, []);

  async function loadClients(options = {}) {
    if (!options.silent) {
      setLoading(true);
    }

    try {
      const result = await api.listClients(connection.id);
      setClients(result.clients || []);
    } catch (error) {
      if (!options.silent) {
        onNotice({ tone: "error", message: error.message });
      }
    } finally {
      if (!options.silent) {
        setLoading(false);
      }
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
    const confirmed = await onConfirm?.({
      title: "Удалить клиента?",
      message: `Клиент «${client.name}» будет удален. Его текущий конфиг больше не сможет подключаться к этому серверу.`,
      confirmLabel: "Удалить",
      tone: "danger"
    });
    if (!confirmed) return;

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
        online: client.online,
        bytesUsed: client.bytesUsed,
        uri: payload.uri || null,
        config: payload.config,
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
                  <div className="client-info" style={{ flex: 1 }}>
                    <div style={{ display: "flex", alignItems: "center", gap: "0.4rem", marginBottom: "0.2rem" }}>
                      <span
                        style={{
                          display: "inline-block",
                          width: "8px",
                          height: "8px",
                          borderRadius: "50%",
                          background: client.online ? "var(--good-color, #10b981)" : "var(--border-color, #d1d5db)",
                          boxShadow: client.online ? "0 0 0 2px rgba(16, 185, 129, 0.2)" : "none"
                        }}
                        title={client.online ? "В сети" : "Офлайн"}
                      />
                      <strong style={{ fontSize: "1rem", lineHeight: "1" }}>{client.name}</strong>
                    </div>
                    <p className="muted-caption" style={{ marginLeft: "0.9rem" }}>Создан: {formatDateTime(client.createdAt)}</p>
                  </div>

                  <div className="client-stats" style={{ flex: 1.2, display: "flex", gap: "1.5rem" }}>
                    <div>
                      <span className="muted-caption" style={{ display: "block", fontSize: "0.65rem", textTransform: "uppercase", letterSpacing: "0.05em", marginBottom: "0.1rem" }}>Активность</span>
                      <strong style={{ fontSize: "0.85rem", color: "var(--text-strong)" }}>{client.online ? "Сейчас" : (client.lastSeen ? formatDateTime(client.lastSeen) : "—")}</strong>
                    </div>
                    <div>
                      <span className="muted-caption" style={{ display: "block", fontSize: "0.65rem", textTransform: "uppercase", letterSpacing: "0.05em", marginBottom: "0.1rem" }}>Трафик</span>
                      <strong style={{ fontSize: "0.85rem", color: "var(--text-strong)" }}>{formatBytes(client.bytesUsed)}</strong>
                    </div>
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

function ConfigSummary({ config }) {
  if (!config || !config.client) return null;
  const { server } = config.client;

  return (
    <div className="config-summary" style={{ display: 'grid', gap: '1rem', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))' }}>
      <div className="fact-card">
        <span className="muted-caption">Адрес сервера</span>
        <strong>{server?.address || '—'}</strong>
      </div>
      <div className="fact-card">
        <span className="muted-caption">Домен</span>
        <strong>{server?.domain || '—'}</strong>
      </div>
      <div className="fact-card">
        <span className="muted-caption">Ключ доступа (PSK)</span>
        <code style={{ fontSize: '0.85rem', color: 'var(--accent-strong)' }}>{server?.psk || '—'}</code>
      </div>
      <div className="fact-card">
        <span className="muted-caption">SOCKS5 прокси</span>
        <strong>{config.client.socks5_listen || '—'}</strong>
      </div>
      <div className="fact-card">
        <span className="muted-caption">HTTP прокси</span>
        <strong>{config.client.http_proxy_listen || '—'}</strong>
      </div>
    </div>
  );
}

function ClientConfigModal({ preview, onClose, onNotice }) {
  function downloadJson() {
    const blob = new Blob([preview.configJson], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `${preview.name.replace(/[^a-zA-Z0-9_-]/g, "_")}.json`;
    anchor.click();
    URL.revokeObjectURL(url);
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
            <span className="eyebrow">Настройки клиента</span>
            <h3>{preview.name}</h3>
            <p className="modal-subtitle">Параметры подключения pp-fallback</p>
          </div>
          <button className="icon-button" onClick={onClose}>
            ×
          </button>
        </div>

        <div className="modal-body scrollable">
          <div className="config-download-block">
            <div className="config-download-info">
              <span className="config-format-badge">URI</span>
              <div>
                <strong>URI подключения</strong>
                <p className="muted-caption">JSON доступен только как скачиваемый файл.</p>
              </div>
            </div>

            <div className="button-group">
              <button className="primary-button primary-button--sm" onClick={downloadJson}>
                Скачать .json
              </button>
            </div>
          </div>

          {preview.uri ? (
            <div className="uri-block" style={{ marginTop: "1rem" }}>
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

          <div style={{ marginTop: '1.5rem' }}>
            <ConfigSummary config={preview.config} />
          </div>
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

  const isRunning = !!data.core.processRunning;
  const isCoreReady = data.core.binaryAvailable && data.core.configValid && isRunning;

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
            message: "Исполняемый файл 'pp' или 'pp-core' не найден. Без него запуск ядра невозможен."
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

function SettingsPage({ bootstrap, onNotice, onConfirm, onRefreshAbout }) {
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [form, setForm] = useState(null);
  const [initialUpdateChannel, setInitialUpdateChannel] = useState("stable");

  useEffect(() => {
    loadSettings();
  }, []);

  async function loadSettings() {
    setLoading(true);
    try {
      const data = await api.getSettings();
      setForm(data);
      setInitialUpdateChannel(data.updateChannel || "stable");
    } catch (error) {
      onNotice({ tone: "error", message: error.message });
    } finally {
      setLoading(false);
    }
  }

  async function handleSubmit(event) {
    event.preventDefault();
    setSubmitting(true);
    const previousChannel = initialUpdateChannel;
    const nextChannel = form.updateChannel || "stable";
    try {
      await api.saveSettings(form);
      setInitialUpdateChannel(nextChannel);
      onNotice({ tone: "success", message: "Настройки сохранены. Используйте кнопку ниже для перезапуска панели." });
      if (nextChannel !== previousChannel) {
        await onRefreshAbout?.({ force: true, silent: true });
        onNotice({
          tone: "warning",
          message: "Канал обновлений изменен. Если для выбранной ветки есть релиз, он появится на странице «О программе» и будет установлен только после вашего подтверждения."
        });
      }
    } catch (error) {
      onNotice({ tone: "error", message: error.message });
    } finally {
      setSubmitting(false);
    }
  }

  async function handleRestart() {
    const confirmed = await onConfirm?.({
      title: "Перезапустить панель?",
      message: "Соединение с PP Web будет временно разорвано. Через несколько секунд браузер перейдет на новый адрес панели.",
      confirmLabel: "Перезапустить",
      tone: "warning"
    });
    if (!confirmed) return;
    try {
      await api.restartPanel();
      onNotice({ tone: "success", message: "Панель перезапускается..." });
      setTimeout(() => {
        window.location.href = previewUrl;
      }, 3000);
    } catch (error) {
      onNotice({ tone: "error", message: error.message });
    }
  }

  if (loading || !form) return <PageSkeleton title="Настройки панели" />;

  // Вычисляем предпросмотр
  const protocol = form.panelHttps ? "https" : "http";
  const domain = form.panelDomain || (bootstrap?.publicIP !== "Unknown" ? bootstrap.publicIP : "127.0.0.1");
  const port = form.panelPort || 4090;
  const prefix = form.panelPrefix ? `/${form.panelPrefix.replace(/^\//, "")}` : "";
  const previewUrl = `${protocol}://${domain}:${port}${prefix}`;

  async function handleUpdateChannelChange(nextChannel) {
    if (nextChannel === form.updateChannel) return;
    const confirmed = await onConfirm?.({
      title: nextChannel === "testing" ? "Перейти на тестовую ветку?" : "Сменить канал обновлений?",
      message: nextChannel === "testing"
        ? "В тестовой ветке могут появиться параметры, которых еще нет у клиентов, или измениться логика подключений. После смены канала проверьте доступные обновления и запускайте установку только когда готовы."
        : "После смены канала набор доступных релизов может измениться. Проверьте обновление на странице «О программе» и запускайте установку только когда готовы.",
      confirmLabel: "Сменить канал",
      tone: "warning"
    });
    if (!confirmed) return;
    setForm({ ...form, updateChannel: nextChannel });
  }

  return (
    <div className="page">
      <section className="settings-grid">
        <article className="surface-card surface-card--wide">
          <div className="surface-card__head">
            <div>
              <span className="eyebrow">Panel</span>
              <h3>Конфигурация доступа</h3>
            </div>
          </div>

          <form onSubmit={handleSubmit} className="auth-form" style={{ maxWidth: "600px" }}>
            <div className="settings-section">
              <h4>Сетевые настройки</h4>
              <div className="input-group">
                <label>Порт панели</label>
                <input
                  type="number"
                  min="1"
                  max="65535"
                  value={form.panelPort || ""}
                  onChange={(e) => setForm({ ...form, panelPort: parseInt(e.target.value) || 4090 })}
                />
              </div>

              <div className="input-group">
                <label>Префикс пути (напр. /panel)</label>
                <input
                  type="text"
                  placeholder="/"
                  value={form.panelPrefix || ""}
                  onChange={(e) => setForm({ ...form, panelPrefix: e.target.value })}
                />
                <p className="muted-caption">Если указать "panel", доступ к системе будет через /panel/</p>
              </div>
            </div>

            <div className="settings-section">
              <h4>Безопасность и HTTPS</h4>
              <div className="input-group">
                <label>Домен (для HTTPS)</label>
                <input
                  type="text"
                  placeholder="panel.example.com"
                  value={form.panelDomain || ""}
                  onChange={(e) => setForm({ ...form, panelDomain: e.target.value })}
                />
                <p className="muted-caption">Требуется для корректной работы сертификатов.</p>
              </div>

              <div className="checkbox-group">
                <label>
                  <input
                    type="checkbox"
                    checked={!!form.panelHttps}
                    onChange={(e) => setForm({ ...form, panelHttps: e.target.checked })}
                  />
                  <span>Включить HTTPS (самоподписанный)</span>
                </label>
              </div>
            </div>

            <div className="settings-section">
              <h4>Обновления</h4>
              <div className="input-group">
                <label>Канал обновлений</label>
                <select
                  value={form.updateChannel || "stable"}
                  onChange={(e) => handleUpdateChannelChange(e.target.value)}
                >
                  <option value="stable">Стабильный (рекомендуется)</option>
                  <option value="testing">Тестовый (Beta)</option>
                </select>
                <p className="muted-caption">
                  На тестовом канале вы будете получать самые новые функции быстрее, но возможна нестабильная работа.
                </p>
              </div>
            </div>

            <div className="settings-preview">
              <span className="eyebrow">Итоговый адрес доступа</span>
              <code>{previewUrl}</code>
            </div>

            <button type="submit" className="primary-button" disabled={submitting}>
              {submitting ? "Сохранение..." : "Сохранить изменения"}
            </button>

            <div style={{ marginTop: "2rem", borderTop: "1px solid var(--border-color)", paddingTop: "1.5rem" }}>
              <h4>Перезапуск панели</h4>
              <p className="muted-caption" style={{ marginBottom: "1rem" }}>
                Нажмите кнопку ниже, чтобы полностью перезапустить веб-интерфейс.
                Это необходимо для применения изменений порта, HTTPS или префикса пути.
              </p>
              <button
                type="button"
                className="ghost-button destructive"
                onClick={handleRestart}
              >
                Перезапустить панель
              </button>
            </div>
          </form>
        </article>
      </section>
    </div>
  );
}

function RoutingMapEditor({ routing, onChange }) {
  const rules = routing.rules || [];
  const activeRules = rules.filter((rule) => rule.value?.trim()).length;
  const draftRules = rules.length - activeRules;
  const defaultPolicy = routing.default_policy || "proxy";

  function updateRouting(nextRules) {
    onChange({ ...routing, rules: nextRules });
  }

  function setPolicy(value) {
    onChange({ ...routing, default_policy: value });
  }

  function addRule(rule = { type: "domain", value: "", policy: "direct", comment: "" }) {
    updateRouting([...rules, rule]);
  }

  function updateRule(index, field, value) {
    updateRouting(rules.map((rule, ruleIndex) => (ruleIndex === index ? { ...rule, [field]: value } : rule)));
  }

  function deleteRule(index) {
    updateRouting(rules.filter((_, ruleIndex) => ruleIndex !== index));
  }

  function moveRule(index, direction) {
    const nextRules = [...rules];
    const swapIndex = index + direction;
    if (swapIndex < 0 || swapIndex >= nextRules.length) return;
    [nextRules[index], nextRules[swapIndex]] = [nextRules[swapIndex], nextRules[index]];
    updateRouting(nextRules);
  }

  return (
    <div className="routing-map-editor">
      <section className="routing-map-panel">
        <div className="routing-map-panel__head">
          <div>
            <strong>Карта маршрутов</strong>
            <span>{activeRules ? `${activeRules} правил настроено` : "Правил пока нет"}</span>
          </div>
          {draftRules ? <span className="routing-draft-badge">{draftRules} черновик</span> : null}
        </div>

        <div className="routing-default-line">
          <span>Если ни одно правило не подошло</span>
          <strong>{POLICY_LABELS[defaultPolicy] || defaultPolicy}</strong>
        </div>

        {activeRules ? (
          <ol className="routing-map-list">
            {rules.map((rule, index) =>
              rule.value?.trim() ? (
                <li key={`${rule.type}-${rule.value}-${index}`}>
                  <span>{index + 1}</span>
                  <strong>{describeRoutingRule(rule)}</strong>
                  <em>{POLICY_LABELS[rule.policy] || rule.policy}</em>
                </li>
              ) : null
            )}
          </ol>
        ) : (
          <p className="routing-empty">
            Сейчас действует только правило "иначе". Добавьте маршрут ниже, чтобы явно задать поведение для домена,
            категории или IP-диапазона.
          </p>
        )}
      </section>

      <section className="routing-control-panel">
        <div className="routing-policy-row">
          <span className="routing-policy-label">Правило "иначе"</span>
          <div className="routing-policy-btns">
            {RULE_POLICIES.map((policy) => (
              <button
                key={policy}
                type="button"
                className={`policy-pill policy-pill--${policy} ${defaultPolicy === policy ? "policy-pill--active" : ""}`}
                onClick={() => setPolicy(policy)}
                title={POLICY_DESCRIPTIONS[policy]}
              >
                {POLICY_LABELS[policy]}
              </button>
            ))}
          </div>
        </div>

        <div className="routing-presets">
          <span className="routing-policy-label">Быстро добавить</span>
          <div className="routing-preset-buttons">
            {ROUTING_PRESETS.map((preset) => (
              <button
                key={preset.label}
                type="button"
                className="ghost-button ghost-button--small"
                onClick={() => addRule({ ...preset.rule })}
              >
                {preset.label}
              </button>
            ))}
          </div>
        </div>
      </section>

      <div className="route-list-editor">
        {rules.map((rule, index) => (
          <article className={`route-row ${rule.value?.trim() ? "" : "route-row--draft"}`} key={index}>
            <div className="route-row__summary">
              <div>
                <span>Маршрут {index + 1}</span>
                <strong>{describeRoutingRule(rule)}</strong>
              </div>
              <em>{rule.value?.trim() ? POLICY_LABELS[rule.policy] || rule.policy : "не сохранится без цели"}</em>
            </div>

            <div className="route-row__controls">
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

              <input
                className="rule-value-input"
                placeholder={
                  rule.type === "geosite" || rule.type === "geoip"
                    ? "ru"
                    : rule.type === "ip_cidr"
                      ? "10.0.0.0/8"
                      : "example.com"
                }
                value={rule.value}
                onChange={(event) => updateRule(index, "value", event.target.value)}
              />

              <select
                className={`rule-select rule-policy-select rule-policy-select--${rule.policy}`}
                value={rule.policy}
                onChange={(event) => updateRule(index, "policy", event.target.value)}
                title={POLICY_DESCRIPTIONS[rule.policy]}
              >
                {RULE_POLICIES.map((policy) => (
                  <option key={policy} value={policy}>
                    {POLICY_LABELS[policy] || policy}
                  </option>
                ))}
              </select>
            </div>

            <p className="rule-help">{RULE_TYPE_HELP[rule.type]}</p>

            <div className="route-row__footer">
              <input
                className="rule-comment-input"
                placeholder="Комментарий"
                value={rule.comment || ""}
                onChange={(event) => updateRule(index, "comment", event.target.value)}
              />
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
          </article>
        ))}
      </div>

      <button type="button" className="ghost-button ghost-button--small rule-add-btn" onClick={() => addRule()}>
        Добавить маршрут
      </button>
    </div>
  );
}

function RoutingRulesEditor({ routing, onChange }) {
  const rules = routing.rules || [];
  const activeRules = rules.filter((rule) => rule.value?.trim()).length;
  const defaultPolicy = routing.default_policy || "proxy";

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
      <div className="routing-summary">
        <strong>{activeRules ? `${activeRules} активных правил` : "Активных правил нет"}</strong>
        <span>
          Остальной трафик: {POLICY_LABELS[defaultPolicy] || defaultPolicy}. Правила проверяются сверху вниз и
          применяются после сохранения подключения.
        </span>
      </div>

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
              title={POLICY_DESCRIPTIONS[policy]}
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
                  title={POLICY_DESCRIPTIONS[rule.policy]}
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

function TagInput({ value, onChange, placeholder }) {
  const [draft, setDraft] = useState("");

  function commitTags(rawValue) {
    const nextTags = dedupeTags([...value, rawValue]);
    if (nextTags.length === value.length) {
      return false;
    }

    onChange(nextTags);
    return true;
  }

  function handleInputChange(event) {
    const rawValue = event.target.value;
    if (!rawValue.includes(",")) {
      setDraft(rawValue);
      return;
    }

    const parts = rawValue.split(",");
    const pending = parts.pop() ?? "";
    const completed = dedupeTags(parts);

    if (completed.length) {
      onChange(dedupeTags([...value, ...completed]));
    }

    setDraft(pending.replace(/^\s+/, ""));
  }

  function handleKeyDown(event) {
    if ((event.key === "Enter" || event.key === "Tab") && draft.trim()) {
      event.preventDefault();
      if (commitTags(draft)) {
        setDraft("");
      }
      return;
    }

    if (event.key === "Backspace" && !draft && value.length) {
      event.preventDefault();
      onChange(value.slice(0, -1));
    }
  }

  function handleBlur() {
    if (!draft.trim()) {
      setDraft("");
      return;
    }

    if (commitTags(draft)) {
      setDraft("");
    }
  }

  function handleRemove(index) {
    onChange(value.filter((_, tagIndex) => tagIndex !== index));
  }

  return (
    <div className="tag-editor">
      <div className="tag-editor__shell">
        {value.map((tag, index) => (
          <span className="tag-editor__tag" key={`${tag}-${index}`}>
            <span>{tag}</span>
            <button type="button" onClick={() => handleRemove(index)} aria-label={`Удалить тег ${tag}`}>
              ×
            </button>
          </span>
        ))}

        <input
          type="text"
          value={draft}
          onChange={handleInputChange}
          onKeyDown={handleKeyDown}
          onBlur={handleBlur}
          placeholder={value.length ? "Добавьте ещё тег" : placeholder}
        />
      </div>
    </div>
  );
}

function ConnectionEditor({ connection, connections, protocols, onClose, onSaved, onNotice }) {
  const defaultRouting = {
    default_policy: "proxy",
    rules: []
  };
  const defaultPort = (() => {
    if (connection?.listen) return connection.listen.split(":").pop();
    const used = new Set((connections || []).map(c => c.listen?.split(":").pop()));
    let p = 8081;
    while (used.has(p.toString())) p++;
    return p.toString();
  })();
  const legacyPublishInterval = connection?.settings?.publish_interval_minutes ?? 60;
  const derivedPublishMinDelay = connection?.settings?.publish_min_delay_minutes ?? Math.max(5, Math.floor((legacyPublishInterval * 3) / 5));
  const derivedPublishMaxDelay =
    connection?.settings?.publish_max_delay_minutes ?? Math.max(derivedPublishMinDelay, Math.floor((legacyPublishInterval * 3) / 2));
  const initialSettings = connection?.settings
    ? {
      ...connection.settings,
      publish_min_delay_minutes: derivedPublishMinDelay,
      publish_max_delay_minutes: derivedPublishMaxDelay,
      publish_batch_size: connection.settings.publish_batch_size ?? 3
    }
    : {
      type: "blog",
      domain: "",
      scraper_keywords: [],
      noise_private_key: "",
      psk: "",
      publish_min_delay_minutes: 15,
      publish_max_delay_minutes: 75,
      publish_batch_size: 3
    };

  const [form, setForm] = useState({
    name: connection?.name || "",
    enabled: connection ? connection.enabled : true,
    protocol: connection?.protocol || protocols[0]?.id || "pp-fallback",
    port: connection?.listen?.split(":").pop() || "8081",
    tag: connection?.tag || "",
    settings: initialSettings
  });
  const [routing, setRouting] = useState(connection?.settings?.routing ?? defaultRouting);
  const [showRouting, setShowRouting] = useState(false);
  const [saving, setSaving] = useState(false);
  const [portStatus, setPortStatus] = useState(null);
  const legacySiteType = form.settings.type && form.settings.type !== "blog" ? form.settings.type : null;
  const siteTypeOptions = legacySiteType
    ? [
      { value: legacySiteType, label: `${getSiteTypeLabel(legacySiteType)} (устарел)`, disabled: true },
      { value: "blog", label: "Новости" }
    ]
    : [{ value: "blog", label: "Новости" }];

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
    const settings = { ...formData.settings };
    delete settings.publish_interval_minutes;
    const normalizedRouting = normalizeRoutingForSave(routing);
    const payload = {
      ...formData,
      tls: connection?.tls ?? null,
      listen: `127.0.0.1:${port}`,
      tag: formData.tag || `tag-${port}-${Math.random().toString(36).substring(2, 8)}`,
      settings: { ...settings, routing: normalizedRouting }
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
                  placeholder="Например: новости-1"
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
                      {siteTypeOptions.map((option) => (
                        <option key={option.value} value={option.value} disabled={option.disabled}>
                          {option.label}
                        </option>
                      ))}
                    </select>
                    {legacySiteType ? (
                      <p className="muted-caption">
                        Для этого подключения сохранён устаревший тип «{getSiteTypeLabel(legacySiteType)}».
                        {" "}После переключения обратно выбрать его уже нельзя.
                      </p>
                    ) : null}
                  </div>

                  <div className="input-group">
                    <label>Теги публикаций</label>
                    <TagInput
                      value={form.settings.scraper_keywords || []}
                      onChange={(keywords) =>
                        setForm({
                          ...form,
                          settings: { ...form.settings, scraper_keywords: keywords }
                        })
                      }
                      placeholder="Жизнь в лесу, новые технологии"
                    />
                    <p className="muted-caption">Запятая или Enter фиксируют тег. Новые статьи будут подбираться по этим тегам.</p>
                  </div>

                  <div className="input-group">
                    <label>Мин. задержка публикации, мин</label>
                    <input
                      type="number"
                      min="1"
                      value={form.settings.publish_min_delay_minutes ?? 15}
                      onChange={(event) =>
                        setForm({
                          ...form,
                          settings: {
                            ...form.settings,
                            publish_min_delay_minutes: Number(event.target.value || 0)
                          }
                        })
                      }
                    />
                    <p className="muted-caption">Новая статья появится не сразу, а после случайной паузы внутри заданного окна.</p>
                  </div>

                  <div className="input-group">
                    <label>Макс. задержка публикации, мин</label>
                    <input
                      type="number"
                      min="1"
                      value={form.settings.publish_max_delay_minutes ?? 75}
                      onChange={(event) =>
                        setForm({
                          ...form,
                          settings: {
                            ...form.settings,
                            publish_max_delay_minutes: Number(event.target.value || 0)
                          }
                        })
                      }
                    />
                  </div>

                  <div className="input-group">
                    <label>Пакет публикации</label>
                    <input
                      type="number"
                      min="1"
                      value={form.settings.publish_batch_size ?? 3}
                      onChange={(event) =>
                        setForm({
                          ...form,
                          settings: {
                            ...form.settings,
                            publish_batch_size: Number(event.target.value || 0)
                          }
                        })
                      }
                    />
                  </div>
                </>
              ) : null}

              <div className="section-divider" />

              <div className="section-header-row">
                <h4>Роутинг клиентов</h4>
                <button
                  type="button"
                  className="ghost-button ghost-button--small"
                  onClick={() => setShowRouting(!showRouting)}
                >
                  {showRouting ? "Скрыть" : "Настроить"}
                </button>
              </div>

              <p className="muted-caption">
                Правила хранятся на сервере и автоматически применяются клиентами этого подключения.
              </p>

              {showRouting ? <RoutingMapEditor routing={routing} onChange={setRouting} /> : null}

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

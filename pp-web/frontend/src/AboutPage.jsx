import { useState } from "react";
import { api } from "./api";

function formatDateTime(value) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  if (date.getUTCFullYear() < 2000) return "—";
  return new Intl.DateTimeFormat("ru-RU", {
    dateStyle: "medium",
    timeStyle: "short"
  }).format(date);
}

function formatBuildDate(value) {
  if (!value || value === "unknown" || value === "none") return "—";
  return value.split("T")[0];
}

function formatUpdateMode(mode) {
  switch (mode) {
    case "service": return "Служба (pp-web-update)";
    case "transient": return "Временная (systemd)";
    case "direct": return "Прямое обновление";
    default: return "Недоступно";
  }
}

async function copyToClipboard(value) {
  if (!value) return false;
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value);
      return true;
    }
  } catch {}
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

function InlineRichText({ text }) {
  const source = text ?? "";
  const tokens = source.split(/(\*\*[^*]+\*\*|https?:\/\/[^\s]+)/g).filter(Boolean);
  return tokens.map((token, index) => {
    if (/^\*\*[^*]+\*\*$/.test(token)) {
      return <strong key={`${token}-${index}`}>{token.slice(2, -2)}</strong>;
    }
    if (/^https?:\/\/[^\s]+$/.test(token)) {
      return (
        <a key={`${token}-${index}`} href={token} target="_blank" rel="noreferrer" style={{ color: "var(--accent-strong)" }}>
          {token}
        </a>
      );
    }
    return <span key={`${token}-${index}`}>{token}</span>;
  });
}

function ReleaseNotes({ body }) {
  const lines = (body || "").split(/\r?\n/);
  const elements = [];
  let listItems = [];

  function flushList() {
    if (!listItems.length) return;
    elements.push(
      <ul key={`list-${elements.length}`} className="release-notes__list" style={{ marginTop: "0.5rem", marginBottom: "1rem" }}>
        {listItems.map((item, index) => (
          <li key={`${item}-${index}`} style={{ marginBottom: "0.4rem" }}>
            <InlineRichText text={item} />
          </li>
        ))}
      </ul>
    );
    listItems = [];
  }

  lines.forEach((line, index) => {
    const trimmed = line.trim();
    if (!trimmed) { flushList(); return; }
    if (/^[-*]\s+/.test(trimmed)) { listItems.push(trimmed.replace(/^[-*]\s+/, "")); return; }
    flushList();
    if (/^#{1,6}\s+/.test(trimmed)) {
      elements.push(
        <h4 key={`heading-${index}`} className="release-notes__heading" style={{ marginTop: "1.5rem", marginBottom: "0.5rem", color: "var(--text-color)", fontSize: "1.1rem" }}>
          {trimmed.replace(/^#{1,6}\s+/, "")}
        </h4>
      );
      return;
    }
    elements.push(
      <p key={`paragraph-${index}`} className="release-notes__paragraph" style={{ marginBottom: "0.75rem", lineHeight: "1.6", color: "var(--text-muted)" }}>
        <InlineRichText text={trimmed} />
      </p>
    );
  });
  flushList();

  if (!elements.length) {
    return <p className="empty-muted" style={{ color: "var(--text-soft)" }}>Описание релиза пока отсутствует.</p>;
  }
  return <div className="release-notes">{elements}</div>;
}

function AboutStatusPill({ tone, children }) {
  return <span className={`status-pill status-pill--${tone}`}>{children}</span>;
}

const IconCopy = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
    <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
  </svg>
);

const IconGithub = ({ size = 24, style }) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" style={style}>
    <path d="M9 19c-5 1.5-5-2.5-7-3m14 6v-3.87a3.37 3.37 0 0 0-.94-2.61c3.14-.35 6.44-1.54 6.44-7A5.44 5.44 0 0 0 20 4.77 5.07 5.07 0 0 0 19.91 1S18.73.65 16 2.48a13.38 13.38 0 0 0-7 0C6.27.65 5.09 1 5.09 1A5.07 5.07 0 0 0 5 4.77a5.44 5.44 0 0 0-1.5 3.78c0 5.42 3.3 6.61 6.44 7A3.37 3.37 0 0 0 9 18.13V22"></path>
  </svg>
);

const IconRefresh = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <polyline points="23 4 23 10 17 10"></polyline>
    <polyline points="1 20 1 14 7 14"></polyline>
    <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"></path>
  </svg>
);

const IconRocket = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="M4.5 16.5c-1.5 1.26-2 5-2 5s3.74-.5 5-2c.71-.84.7-2.13-.09-2.91a2.18 2.18 0 0 0-2.91-.09z"></path>
    <path d="m12 15-3-3a22 22 0 0 1 2-3.95A12.88 12.88 0 0 1 22 2c0 2.72-.78 7.5-6 11a22.35 22.35 0 0 1-4 2z"></path>
    <path d="M9 12H4s.55-3.03 2-4c1.62-1.08 5 0 5 0"></path>
    <path d="M12 15v5s3.03-.55 4-2c1.08-1.62 0-5 0-5"></path>
  </svg>
);

export default function AboutPage({ data, loading, error, onRefresh, onNotice }) {
  const [submitting, setSubmitting] = useState(false);

  const release = data?.release;
  const app = data?.app;
  const github = data?.github;
  const update = data?.update;
  
  const updateState = update?.status?.state;
  const updateBusy = submitting || updateState === "queued" || updateState === "running";
  const busy = loading || updateBusy;
  const visibleError = error || release?.error;
  
  const githubUrl = github?.url || "https://github.com/vakaka1/pp";
  const releasesUrl = github?.releasesUrl || `${githubUrl}/releases`;

  async function handleUpdate() {
    setSubmitting(true);
    try {
      const payload = await api.startAboutUpdate();
      onNotice({ tone: "success", message: payload.message || "Обновление успешно запущено." });
      await onRefresh?.({ force: true });
    } catch (err) {
      onNotice({ tone: "error", message: err.message });
    } finally {
      setSubmitting(false);
    }
  }

  async function handleCopy(value, successMessage) {
    const copied = await copyToClipboard(value);
    onNotice({ tone: copied ? "success" : "error", message: copied ? successMessage : "Не удалось скопировать путь." });
  }

  if (loading && !data) {
    return (
      <div className="page" style={{ animation: "fadeInUp 380ms ease" }}>
        <div className="skeleton-box" style={{ height: "180px", borderRadius: "var(--radius-xl)" }} />
        <div className="skeleton-box" style={{ height: "260px", borderRadius: "var(--radius-lg)" }} />
        <div className="skeleton-box" style={{ height: "300px", borderRadius: "var(--radius-lg)" }} />
      </div>
    );
  }

  let updateStatusColor = "neutral";
  let updateStatusText = "Проверяем...";
  if (release?.updateAvailable) {
    updateStatusColor = release.indicatorTone === "danger" ? "bad" : "warning";
    updateStatusText = "Доступно обновление";
  } else if (release && !release.error) {
    updateStatusColor = "good";
    updateStatusText = "Система актуальна";
  } else if (release?.error) {
    updateStatusColor = "bad";
    updateStatusText = "Ошибка проверки";
  }

  return (
    <div className="page" style={{ animation: "fadeInUp 380ms ease" }}>
      
      {/* Элегантная шапка (Hero) */}
      <div style={{
        display: "flex", flexWrap: "wrap", gap: "1.5rem", justifyContent: "space-between", alignItems: "center",
        padding: "2.5rem 2.8rem", borderRadius: "var(--radius-xl)",
        background: "linear-gradient(135deg, var(--surface-color), var(--surface-strong))",
        border: "1px solid var(--border-color)", boxShadow: "var(--shadow-sm)",
        position: "relative", overflow: "hidden"
      }}>
        <div style={{ position: "absolute", top: "-50%", right: "-10%", width: "60%", height: "200%", background: "radial-gradient(circle, var(--ambient-1), transparent 70%)", opacity: 0.6, pointerEvents: "none" }} />
        <div style={{ position: "absolute", bottom: "-50%", left: "-10%", width: "50%", height: "150%", background: "radial-gradient(circle, var(--ambient-3), transparent 70%)", opacity: 0.3, pointerEvents: "none" }} />
        
        <div style={{ position: "relative", zIndex: 1 }}>
          <div style={{ display: "flex", alignItems: "center", gap: "1.2rem", marginBottom: "0.6rem" }}>
             <h1 style={{ fontSize: "3.2rem", margin: 0, fontFamily: "var(--font-display)", letterSpacing: "-0.03em", color: "var(--text-color)" }}>PP Web</h1>
             <span style={{ padding: "0.45rem 1rem", background: "var(--surface-accent)", border: "1px solid var(--border-color)", borderRadius: "var(--radius-pill)", fontSize: "0.85rem", fontWeight: "800", color: "var(--text-soft)", backdropFilter: "blur(10px)" }}>
                v{app?.version || "—"}
             </span>
          </div>
          <p style={{ color: "var(--text-muted)", fontSize: "1.1rem", margin: 0, maxWidth: "38rem", lineHeight: "1.5" }}>
            Профессиональный интерфейс для управления вашим PP-сервером. Единый центр для контроля за конфигурацией, туннелями и версиями системы.
          </p>
        </div>

        <div style={{ display: "flex", gap: "1rem", position: "relative", zIndex: 1, flexWrap: "wrap" }}>
           <button className="ghost-button" onClick={() => onRefresh?.({ force: true })} disabled={busy}>
             <IconRefresh /> Проверить обновления
           </button>
           {release?.updateAvailable && (
             <button className="primary-button" onClick={handleUpdate} disabled={busy || !update?.canStart}>
               <IconRocket /> {updateBusy ? "Запуск обновления..." : `Установить ${release.latestVersion}`}
             </button>
           )}
        </div>
      </div>

      {visibleError && (
        <div className="inline-banner inline-banner--warning" style={{ marginTop: 0 }}>
          <strong>Внимание:</strong> {visibleError}
        </div>
      )}

      {/* Единый блок "Информация о системе" */}
      <article className="surface-card" style={{ padding: "2rem" }}>
        <div className="surface-card__head" style={{ marginBottom: "1.8rem" }}>
          <h3 style={{ margin: 0, fontSize: "1.5rem" }}>Информация о системе</h3>
          <AboutStatusPill tone={updateStatusColor}>{updateStatusText}</AboutStatusPill>
        </div>
        
        <div className="detail-grid" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(260px, 1fr))", gap: "1.2rem" }}>
          <div className="detail-card" style={{ background: "transparent", border: "1px solid var(--border-color)", padding: "1.4rem" }}>
            <span style={{ fontSize: "0.75rem", color: "var(--text-soft)", textTransform: "uppercase", letterSpacing: "0.1em" }}>Текущая версия</span>
            <strong style={{ fontSize: "1.5rem", marginTop: "0.5rem" }}>{app?.version || "—"}</strong>
            <p style={{ marginTop: "0.5rem", fontSize: "0.9rem", color: "var(--text-muted)", lineHeight: "1.5" }}>
              Сборка от {formatBuildDate(app?.buildDate)}<br/>
              Коммит: <code style={{ background: "none", padding: 0, color: "var(--text-color)" }}>{app?.gitCommit ? app.gitCommit.substring(0, 7) : "—"}</code>
            </p>
          </div>

          <div className="detail-card" style={{ background: "transparent", border: "1px solid var(--border-color)", padding: "1.4rem" }}>
            <span style={{ fontSize: "0.75rem", color: "var(--text-soft)", textTransform: "uppercase", letterSpacing: "0.1em" }}>Последний релиз</span>
            <strong style={{ fontSize: "1.5rem", marginTop: "0.5rem" }}>{release?.latestVersion || "—"}</strong>
            <p style={{ marginTop: "0.5rem", fontSize: "0.9rem", color: "var(--text-muted)", lineHeight: "1.5" }}>
              Опубликован {formatDateTime(release?.latestPublishedAt)}
            </p>
          </div>

          <div className="detail-card" style={{ background: "transparent", border: "1px solid var(--border-color)", padding: "1.4rem" }}>
            <span style={{ fontSize: "0.75rem", color: "var(--text-soft)", textTransform: "uppercase", letterSpacing: "0.1em" }}>Состояние обновления</span>
            <strong style={{ fontSize: "1.2rem", marginTop: "0.5rem" }}>
              {updateState === "running" ? "В процессе установки..." : updateState === "queued" ? "В очереди на установку..." : release?.updateAvailable ? "Доступна новая версия" : "Обновления не требуются"}
            </strong>
            <p style={{ marginTop: "0.5rem", fontSize: "0.9rem", color: "var(--text-muted)", lineHeight: "1.5" }}>
              Режим: {formatUpdateMode(update?.mode)}
              {update?.status?.message && <><br/><span style={{ color: "var(--accent-strong)" }}>{update.status.message}</span></>}
            </p>
          </div>
        </div>

        {release?.updateAvailable && !update?.canStart && (
          <div className="inline-banner inline-banner--warning" style={{ fontSize: "0.9rem", padding: "1rem 1.2rem", marginTop: "1.5rem" }}>
            Автообновление недоступно: проверьте наличие службы <strong>pp-web-update</strong> или права на директорию установки.
          </div>
        )}

        <div style={{ marginTop: "2.5rem", paddingTop: "1.8rem", borderTop: "1px solid var(--border-color)" }}>
           <h4 style={{ margin: "0 0 1.2rem", fontSize: "1.1rem", color: "var(--text-color)" }}>Пути установки</h4>
           <div style={{ display: "flex", flexDirection: "column", gap: "0.85rem" }}>
             <div style={{ display: "flex", alignItems: "center", gap: "1rem", background: "var(--surface-accent)", padding: "0.85rem 1.2rem", borderRadius: "var(--radius-md)" }}>
               <span style={{ fontSize: "0.9rem", color: "var(--text-soft)", minWidth: "100px" }}>Бинарник</span>
               <code style={{ flex: 1, background: "transparent", padding: 0, fontSize: "0.9rem", color: "var(--text-color)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{app?.binaryPath || "—"}</code>
               <button className="icon-button icon-button--tiny" onClick={() => handleCopy(app?.binaryPath, "Путь к бинарнику скопирован")}><IconCopy /></button>
             </div>
             <div style={{ display: "flex", alignItems: "center", gap: "1rem", background: "var(--surface-accent)", padding: "0.85rem 1.2rem", borderRadius: "var(--radius-md)" }}>
               <span style={{ fontSize: "0.9rem", color: "var(--text-soft)", minWidth: "100px" }}>Frontend</span>
               <code style={{ flex: 1, background: "transparent", padding: 0, fontSize: "0.9rem", color: "var(--text-color)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{app?.frontendDist || "—"}</code>
               <button className="icon-button icon-button--tiny" onClick={() => handleCopy(app?.frontendDist, "Путь к frontend скопирован")}><IconCopy /></button>
             </div>
           </div>
        </div>
      </article>

      {/* Описание релиза */}
      <article className="surface-card" style={{ padding: "2rem" }}>
        <div className="surface-card__head" style={{ borderBottom: "1px solid var(--border-color)", paddingBottom: "1.5rem", marginBottom: "1.5rem" }}>
          <h3 style={{ margin: 0, fontSize: "1.5rem" }}>Заметки к версии {release?.latestVersion || ""}</h3>
          <span style={{ color: "var(--text-soft)", fontSize: "0.95rem" }}>
             {release?.latestName ? `«${release.latestName}»` : ""}
          </span>
        </div>
        <div style={{ padding: "0 0.5rem" }}>
          <ReleaseNotes body={release?.latestBody} />
        </div>
      </article>

      {/* Рекламный баннер GitHub (В самом низу) */}
      <section style={{
        marginTop: "1.5rem",
        padding: "3.5rem 2rem",
        borderRadius: "var(--radius-xl)",
        background: "linear-gradient(145deg, #0d1117, #161b22)",
        border: "1px solid rgba(240, 246, 252, 0.08)",
        color: "#c9d1d9",
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        textAlign: "center",
        boxShadow: "0 30px 60px rgba(0, 0, 0, 0.25)",
        position: "relative",
        overflow: "hidden"
      }}>
        <div style={{ position: "absolute", top: 0, left: "50%", transform: "translateX(-50%)", width: "80%", height: "1px", background: "radial-gradient(circle, rgba(88, 166, 255, 0.4), transparent 70%)" }} />
        
        <IconGithub size={54} style={{ color: "#f0f6fc", marginBottom: "1.2rem" }} />
        <h2 style={{ color: "#f0f6fc", fontSize: "2.2rem", marginBottom: "0.8rem", fontFamily: "var(--font-sans)", fontWeight: "800", letterSpacing: "-0.02em" }}>Open Source Project</h2>
        <p style={{ maxWidth: "36rem", fontSize: "1.1rem", lineHeight: "1.6", color: "#8b949e", marginBottom: "2.5rem" }}>
          PP Web — это прозрачный и открытый инструмент. Весь исходный код, история изменений, дискуссии и релизы доступны в нашем официальном репозитории на GitHub.
        </p>
        <div className="button-group" style={{ justifyContent: "center", gap: "1rem" }}>
          <a href={githubUrl} target="_blank" rel="noreferrer" style={{
            display: "inline-flex", alignItems: "center", gap: "0.5rem",
            padding: "1rem 1.8rem", borderRadius: "var(--radius-pill)",
            background: "#f0f6fc", color: "#0d1117", fontWeight: "700", fontSize: "0.95rem", textDecoration: "none",
            transition: "transform 0.2s"
          }} onMouseOver={(e) => e.currentTarget.style.transform = "translateY(-2px)"} onMouseOut={(e) => e.currentTarget.style.transform = "none"}>
            Перейти в репозиторий
          </a>
          <a href={releasesUrl} target="_blank" rel="noreferrer" style={{
            display: "inline-flex", alignItems: "center", gap: "0.5rem",
            padding: "1rem 1.8rem", borderRadius: "var(--radius-pill)",
            background: "rgba(240, 246, 252, 0.08)", color: "#c9d1d9", fontWeight: "600", fontSize: "0.95rem", textDecoration: "none",
            border: "1px solid rgba(240, 246, 252, 0.15)",
            transition: "background 0.2s"
          }} onMouseOver={(e) => e.currentTarget.style.background = "rgba(240, 246, 252, 0.12)"} onMouseOut={(e) => e.currentTarget.style.background = "rgba(240, 246, 252, 0.08)"}>
            Все релизы
          </a>
        </div>
      </section>

    </div>
  );
}

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

export default function AboutPage({ data, loading, error, onRefresh, onNotice, onConfirm }) {
  const [submitting, setSubmitting] = useState(false);

  const release = data?.release;
  const app = data?.app;
  const github = data?.github;
  const update = data?.update;
  
  const updateState = update?.status?.state;
  const updateBusy = submitting || updateState === "queued" || updateState === "running";
  const busy = loading || updateBusy;
  const visibleError = error || release?.error;
  const canRollback = Boolean(update?.canRollback);
  
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

  async function handleRollback() {
    const confirmed = await onConfirm?.({
      title: "Откатить обновление?",
      message: "Будут восстановлены бинарные файлы и frontend из резервных копий .bak. Панель и ядро PP могут кратко перезапуститься.",
      confirmLabel: "Откатить",
      tone: "danger"
    });
    if (!confirmed) return;
    setSubmitting(true);
    try {
      const payload = await api.rollback();
      onNotice({ tone: "success", message: payload.message || "Откат запущен." });
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
    <div className="page fade-in">
      
      {/* Элегантная шапка (Hero) */}
      <div className="about-hero">
        <div className="about-hero__aurora about-hero__aurora--one" />
        <div className="about-hero__aurora about-hero__aurora--two" />
        
        <div className="about-hero__content">
          <div className="about-hero__title-row">
             <h1 className="about-hero__title">PP Web</h1>
             <span className="about-hero__version">
                v{app?.version || "—"}
             </span>
          </div>
          <p className="about-hero__copy">
            Профессиональный интерфейс для управления вашим PP-сервером. Единый центр для контроля за конфигурацией, туннелями и версиями системы.
          </p>
        </div>

        <div className="about-hero__actions">
           <button className="ghost-button" onClick={() => onRefresh?.({ force: true })} disabled={busy}>
             <IconRefresh /> Проверить обновления
           </button>
           {canRollback ? (
             <button className="ghost-button" onClick={handleRollback} disabled={busy} title="Откат к предыдущей версии из .bak файлов">
               Откатить изменения
             </button>
           ) : null}
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
      <article className="surface-card about-card">
        <div className="surface-card__head">
          <h3>Информация о системе</h3>
          <AboutStatusPill tone={updateStatusColor}>{updateStatusText}</AboutStatusPill>
        </div>
        
        <div className="detail-grid about-grid">
          <div className="detail-card about-detail-card">
            <span className="about-detail-card__label">Текущая версия</span>
            <strong className="about-detail-card__value">{app?.version || "—"}</strong>
            <p className="about-detail-card__copy">
              Сборка от {formatBuildDate(app?.buildDate)}<br/>
              Коммит: <code>{app?.gitCommit ? app.gitCommit.substring(0, 7) : "—"}</code>
            </p>
          </div>

          <div className="detail-card about-detail-card">
            <span className="about-detail-card__label">Последний релиз</span>
            <strong className="about-detail-card__value">{release?.latestVersion || "—"}</strong>
            <p className="about-detail-card__copy">
              Опубликован {formatDateTime(release?.latestPublishedAt)}
            </p>
          </div>

          <div className="detail-card about-detail-card">
            <span className="about-detail-card__label">Состояние обновления</span>
            <strong className="about-detail-card__value-small">
              {updateState === "running" ? "В процессе установки..." : updateState === "queued" ? "В очереди на установку..." : release?.updateAvailable ? "Доступна новая версия" : "Обновления не требуются"}
            </strong>
            <p className="about-detail-card__copy">
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

        <div className="about-paths">
           <h4>Пути установки</h4>
           <div className="about-paths__list">
             <div className="about-paths__item">
               <span className="about-paths__label">Бинарник</span>
               <code className="about-paths__code">{app?.binaryPath || "—"}</code>
               <button className="icon-button icon-button--tiny" onClick={() => handleCopy(app?.binaryPath, "Путь к бинарнику скопирован")}><IconCopy /></button>
             </div>
             <div className="about-paths__item">
               <span className="about-paths__label">Frontend</span>
               <code className="about-paths__code">{app?.frontendDist || "—"}</code>
               <button className="icon-button icon-button--tiny" onClick={() => handleCopy(app?.frontendDist, "Путь к frontend скопирован")}><IconCopy /></button>
             </div>
           </div>
        </div>
      </article>

      {/* Описание релиза */}
      <article className="surface-card about-card">
        <div className="surface-card__head about-release-head">
          <h3>Заметки к версии {release?.latestVersion || ""}</h3>
          <span className="about-release-tag">
             {release?.latestName ? `«${release.latestName}»` : ""}
          </span>
        </div>
        <div className="about-release-notes">
          <ReleaseNotes body={release?.latestBody} />
        </div>
      </article>

      {/* Рекламный баннер GitHub (В самом низу) */}
      <section className="about-github-promo">
        <div className="about-github-promo__line" />
        
        <IconGithub size={54} className="about-github-promo__icon" />
        <h2 className="about-github-promo__title">Open Source Project</h2>
        <p className="about-github-promo__copy">
          PP Web — это прозрачный и открытый инструмент. Весь исходный код, история изменений, дискуссии и релизы доступны в нашем официальном репозитории на GitHub.
        </p>
        <div className="button-group about-github-promo__actions">
          <a href={githubUrl} target="_blank" rel="noreferrer" className="about-github-promo__link about-github-promo__link--primary">
            Перейти в репозиторий
          </a>
          <a href={releasesUrl} target="_blank" rel="noreferrer" className="about-github-promo__link about-github-promo__link--secondary">
            Все релизы
          </a>
        </div>
      </section>

    </div>
  );
}

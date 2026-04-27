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
  if (!value) return "—";
  if (value === "unknown" || value === "none") return "—";
  return value.split("T")[0];
}

function getUpdateVisual(release) {
  if (!release) {
    return {
      pillTone: "neutral",
      cardTone: "warning",
      label: "Проверяем",
      summary: "Информация о релизах загружается."
    };
  }

  if (release.error && !release.updateAvailable) {
    return {
      pillTone: "warning",
      cardTone: "warning",
      label: "Проверка недоступна",
      summary: "Не удалось получить актуальные данные GitHub Releases."
    };
  }

  if (!release?.updateAvailable) {
    return {
      pillTone: "good",
      cardTone: "good",
      label: "Актуально",
      summary: "На сервере уже стоит последний релиз."
    };
  }

  if (release.indicatorTone === "danger") {
    return {
      pillTone: "bad",
      cardTone: "bad",
      label: "Крупное обновление",
      summary: "Изменилась основная или минорная версия. Стоит проверить заметки к релизу."
    };
  }

  return {
    pillTone: "warning",
    cardTone: "warning",
    label: "Доступно обновление",
    summary: "Доступен патч-релиз с изменениями внутри текущей ветки."
  };
}

function formatUpdateMode(mode) {
  switch (mode) {
    case "service":
      return "служба pp-web-update";
    case "transient":
      return "временная systemd-задача";
    case "direct":
      return "прямое обновление";
    default:
      return "недоступно";
  }
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

function InlineRichText({ text }) {
  const source = text ?? "";
  const tokens = source.split(/(\*\*[^*]+\*\*|https?:\/\/[^\s]+)/g).filter(Boolean);

  return tokens.map((token, index) => {
    if (/^\*\*[^*]+\*\*$/.test(token)) {
      return <strong key={`${token}-${index}`}>{token.slice(2, -2)}</strong>;
    }

    if (/^https?:\/\/[^\s]+$/.test(token)) {
      return (
        <a key={`${token}-${index}`} href={token} target="_blank" rel="noreferrer">
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
      <ul key={`list-${elements.length}`} className="release-notes__list">
        {listItems.map((item, index) => (
          <li key={`${item}-${index}`}>
            <InlineRichText text={item} />
          </li>
        ))}
      </ul>
    );
    listItems = [];
  }

  lines.forEach((line, index) => {
    const trimmed = line.trim();
    if (!trimmed) {
      flushList();
      return;
    }

    if (/^[-*]\s+/.test(trimmed)) {
      listItems.push(trimmed.replace(/^[-*]\s+/, ""));
      return;
    }

    flushList();

    if (/^#{1,6}\s+/.test(trimmed)) {
      elements.push(
        <h4 key={`heading-${index}`} className="release-notes__heading">
          {trimmed.replace(/^#{1,6}\s+/, "")}
        </h4>
      );
      return;
    }

    elements.push(
      <p key={`paragraph-${index}`} className="release-notes__paragraph">
        <InlineRichText text={trimmed} />
      </p>
    );
  });

  flushList();

  if (!elements.length) {
    return <p className="empty-muted">Описание релиза пока отсутствует.</p>;
  }

  return <div className="release-notes">{elements}</div>;
}

function AboutStatusPill({ tone, children }) {
  return <span className={`status-pill status-pill--${tone}`}>{children}</span>;
}

export default function AboutPage({ data, loading, error, onRefresh, onNotice }) {
  const [submitting, setSubmitting] = useState(false);

  const release = data?.release;
  const app = data?.app;
  const github = data?.github;
  const update = data?.update;
  const updateVisual = getUpdateVisual(release);
  const updateState = update?.status?.state;
  const updateBusy = submitting || updateState === "queued" || updateState === "running";
  const busy = loading || updateBusy;
  const visibleError = error || release?.error;
  const githubUrl = github?.url || "https://github.com/vakaka1/pp";
  const releasesUrl = github?.releasesUrl || `${githubUrl}/releases`;
  const issuesUrl = github?.issuesUrl || `${githubUrl}/issues`;

  async function handleUpdate() {
    setSubmitting(true);

    try {
      const payload = await api.startAboutUpdate();
      onNotice({
        tone: "success",
        message: payload.message || "Обновление запущено. Панель применит новый релиз автоматически."
      });
      await onRefresh?.({ force: true });
    } catch (requestError) {
      onNotice({
        tone: "error",
        message: requestError.message
      });
    } finally {
      setSubmitting(false);
    }
  }

  async function handleCopy(value, successMessage) {
    const copied = await copyToClipboard(value);
    onNotice({
      tone: copied ? "success" : "error",
      message: copied ? successMessage : "Не удалось скопировать значение."
    });
  }

  if (loading && !data) {
    return (
      <div className="page">
        <section className="page-hero page-hero--about page-hero--loading">
          <div className="page-hero__copy">
            <span className="eyebrow skeleton-text">О программе</span>
            <h2 className="skeleton-text">PP Web</h2>
            <p className="skeleton-text">Загружаем информацию о релизе и состоянии обновлений.</p>
          </div>
        </section>

        <div className="settings-grid">
          <div className="skeleton-box" style={{ height: "260px" }} />
          <div className="skeleton-box" style={{ height: "260px" }} />
        </div>

        <div className="skeleton-box" style={{ height: "360px" }} />
      </div>
    );
  }

  return (
    <div className="page">
      <section className="page-hero page-hero--about">
        <div className="page-hero__copy">
          <span className="eyebrow">PP Web</span>
          <h2>О программе</h2>
          <p>{app?.description || "PP Web управляет сервером PP и обновляется прямо из веб-интерфейса."}</p>

          <div className="page-hero__actions">
            {release?.updateAvailable ? (
              <button
                className={`primary-button ${release.indicatorTone === "danger" ? "danger" : "warning"}`}
                onClick={handleUpdate}
                disabled={busy || !update?.canStart}
              >
                {updateBusy
                  ? "Обновление запущено..."
                  : `Обновить до ${release.latestVersion || "последней версии"}`}
              </button>
            ) : (
              <button className="ghost-button" onClick={() => onRefresh?.({ force: true })} disabled={busy}>
                Проверить обновления
              </button>
            )}

            <button className="ghost-button" onClick={() => onRefresh?.({ force: true })} disabled={busy}>
              Обновить данные
            </button>

            <a className="ghost-button" href={githubUrl} target="_blank" rel="noreferrer">
              Открыть GitHub
            </a>
          </div>

          {visibleError ? <div className="inline-banner inline-banner--warning">{visibleError}</div> : null}
        </div>

        <div className="page-hero__aside">
          <div className="hero-slab about-hero-slab">
            <div className={`hero-slab__item about-summary-card about-summary-card--${updateVisual.cardTone}`}>
              <span>Состояние</span>
              <strong>{updateVisual.label}</strong>
              <p>{release?.statusLabel || updateVisual.summary}</p>
            </div>

            <div className="hero-slab__item">
              <span>Текущая версия</span>
              <strong>{app?.version || release?.currentVersion || "—"}</strong>
              <p>Сборка от {formatBuildDate(app?.buildDate)}</p>
            </div>

            <div className="hero-slab__item">
              <span>Последний релиз</span>
              <strong>{release?.latestVersion || "—"}</strong>
              <p>{formatDateTime(release?.latestPublishedAt)}</p>
            </div>
          </div>
        </div>
      </section>

      <section className="settings-grid">
        <article className="surface-card github-showcase">
          <div className="surface-card__head">
            <div>
              <span className="eyebrow">GitHub</span>
              <h3>Репозиторий проекта</h3>
            </div>
            <AboutStatusPill tone="neutral">main</AboutStatusPill>
          </div>

          <div className="github-showcase__shell">
            <div className="github-showcase__topbar">
              <span className="github-showcase__dot" />
              <span className="github-showcase__dot" />
              <span className="github-showcase__dot" />
              <code>{github?.repository || "vakaka1/pp"}</code>
            </div>

            <div className="github-showcase__content">
              <div className="github-showcase__path">
                <span className="github-showcase__owner">vakaka1</span>
                <span className="github-showcase__slash">/</span>
                <span className="github-showcase__repo">pp</span>
              </div>

              <p>
                Исходный код, релизы, installer-скрипты и история изменений находятся в одном месте. Отсюда же
                берется описание последнего релиза для этой страницы.
              </p>

              <div className="github-showcase__chips">
                <span className="github-chip">releases</span>
                <span className="github-chip">install-server.sh</span>
                <span className="github-chip">pp-web</span>
              </div>

              <div className="button-group button-group--wrap">
                <a className="ghost-button ghost-button--small" href={githubUrl} target="_blank" rel="noreferrer">
                  Репозиторий
                </a>
                <a
                  className="ghost-button ghost-button--small"
                  href={releasesUrl}
                  target="_blank"
                  rel="noreferrer"
                >
                  Релизы
                </a>
                <a className="ghost-button ghost-button--small" href={issuesUrl} target="_blank" rel="noreferrer">
                  Issues
                </a>
              </div>
            </div>
          </div>
        </article>

        <article className="surface-card">
          <div className="surface-card__head">
            <div>
              <span className="eyebrow">PP Web</span>
              <h3>Сведения о панели</h3>
            </div>
            <AboutStatusPill tone={updateVisual.pillTone}>{app?.version || "—"}</AboutStatusPill>
          </div>

          <dl className="details-list">
            <div className="detail-row">
              <dt>Версия</dt>
              <dd>{app?.version || release?.currentVersion || "—"}</dd>
            </div>
            <div className="detail-row">
              <dt>Дата сборки</dt>
              <dd>{formatBuildDate(app?.buildDate)}</dd>
            </div>
            <div className="detail-row">
              <dt>Коммит</dt>
              <dd>{app?.gitCommit || "—"}</dd>
            </div>
            <div className="detail-row">
              <dt>Проверка релизов</dt>
              <dd>{formatDateTime(release?.checkedAt)}</dd>
            </div>
            <div className="detail-row">
              <dt>Режим обновления</dt>
              <dd>{formatUpdateMode(update?.mode)}</dd>
            </div>
          </dl>

          <div className="detail-grid about-detail-grid">
            <div className="detail-card">
              <span>Бинарник</span>
              <strong>{app?.binaryPath || "—"}</strong>
              <p>
                Текущий путь исполняемого файла <code>pp-web</code>.
              </p>
            </div>
            <div className="detail-card">
              <span>Frontend</span>
              <strong>{app?.frontendDist || "—"}</strong>
              <p>Папка, из которой сервер отдает статические файлы интерфейса.</p>
            </div>
          </div>

          <div className="button-group button-group--wrap">
            <button
              className="ghost-button ghost-button--small"
              onClick={() => handleCopy(app?.binaryPath, "Путь к бинарнику pp-web скопирован.")}
            >
              Скопировать путь к бинарнику
            </button>
            <button
              className="ghost-button ghost-button--small"
              onClick={() => handleCopy(app?.frontendDist, "Путь к frontend dist скопирован.")}
            >
              Скопировать путь к frontend
            </button>
          </div>
        </article>
      </section>

      <section className="settings-grid">
        <article className="surface-card">
          <div className="surface-card__head">
            <div>
              <span className="eyebrow">Обновление</span>
              <h3>Состояние обновления</h3>
            </div>
            <AboutStatusPill
              tone={
                updateState === "success"
                  ? "good"
                  : updateState === "error"
                    ? "bad"
                    : updateState === "queued" || updateState === "running"
                      ? "warning"
                      : "neutral"
              }
            >
              {updateState || "idle"}
            </AboutStatusPill>
          </div>

          <div className="system-info-mini system-info-mini--soft">
            <div className="info-row">
              <span>Статус</span>
              <strong>{update?.status?.message || "Обновление не запускалось."}</strong>
            </div>
            <div className="info-row">
              <span>Целевая версия</span>
              <strong>{update?.status?.targetVersion ? update.status.targetVersion.replace(/^v/, "") : "—"}</strong>
            </div>
            <div className="info-row">
              <span>Запуск</span>
              <strong>{formatDateTime(update?.status?.startedAt)}</strong>
            </div>
            <div className="info-row">
              <span>Завершение</span>
              <strong>{formatDateTime(update?.status?.finishedAt)}</strong>
            </div>
          </div>

          {release?.updateAvailable && !update?.canStart ? (
            <div className="inline-banner inline-banner--warning">
              Интерфейсное обновление недоступно. Нужна служба <code>pp-web-update</code> или права на запись в
              каталоги установки.
            </div>
          ) : null}
        </article>

        <article className="surface-card">
          <div className="surface-card__head">
            <div>
              <span className="eyebrow">Релиз</span>
              <h3>Последний релиз</h3>
            </div>
            <AboutStatusPill tone={updateVisual.pillTone}>{release?.latestVersion || "—"}</AboutStatusPill>
          </div>

          <dl className="details-list">
            <div className="detail-row">
              <dt>Название</dt>
              <dd>{release?.latestName || "—"}</dd>
            </div>
            <div className="detail-row">
              <dt>Опубликован</dt>
              <dd>{formatDateTime(release?.latestPublishedAt)}</dd>
            </div>
            <div className="detail-row">
              <dt>Текущий статус</dt>
              <dd>{release?.statusLabel || "—"}</dd>
            </div>
          </dl>

          <div className="button-group button-group--wrap">
            {release?.latestUrl ? (
              <a className="ghost-button ghost-button--small" href={release.latestUrl} target="_blank" rel="noreferrer">
                Страница релиза
              </a>
            ) : null}
            {release?.updateAvailable ? (
              <button className="primary-button primary-button--sm" onClick={handleUpdate} disabled={busy || !update?.canStart}>
                {updateBusy ? "Запускаем..." : "Установить релиз"}
              </button>
            ) : null}
          </div>
        </article>
      </section>

      <article className="surface-card">
        <div className="surface-card__head">
          <div>
            <span className="eyebrow">Заметки</span>
            <h3>Описание последнего релиза</h3>
          </div>
          <AboutStatusPill tone="neutral">{release?.latestVersion || "—"}</AboutStatusPill>
        </div>

        <ReleaseNotes body={release?.latestBody} />
      </article>
    </div>
  );
}

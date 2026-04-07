/**
 * runtime/log-panel.ts — Live backend log viewer for demo split-pane
 *
 * Polls /api/logs?since=N every 500ms to show backend activity:
 * search queries, fetch, AI prompts/responses, cache hits, layout decisions.
 */

const CAT_COLORS: Record<string, string> = {
  search:   "#58a6ff",
  fetch:    "#7ee787",
  ai:       "#d2a8ff",
  cache:    "#ffa657",
  pipeline: "#ff7b72",
  layout:   "#79c0ff",
};

const CAT_ICONS: Record<string, string> = {
  search:   "SEARCH",
  fetch:    "FETCH",
  ai:       "AI",
  cache:    "CACHE",
  pipeline: "PIPE",
  layout:   "LAYOUT",
};

interface LogEntry {
  seq: number;
  ts: number;
  cat: string;
  msg: string;
  detail?: string;
}

let logContainer: HTMLElement | null = null;
let autoScroll = true;
let lastSeq = 0;

function formatTime(): string {
  const d = new Date();
  return d.toLocaleTimeString("en-US", { hour12: false, hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function renderEntry(entry: LogEntry): HTMLElement {
  const div = document.createElement("div");
  div.className = `log-entry log-${entry.cat}`;

  const color = CAT_COLORS[entry.cat] || "#8b949e";
  const icon = CAT_ICONS[entry.cat] || entry.cat.toUpperCase();

  div.innerHTML = `<span class="log-time">${formatTime()}</span>`
    + `<span class="log-cat" style="color:${color}">[${icon}]</span>`
    + `<span class="log-msg">${escapeHtml(entry.msg)}</span>`;

  if (entry.detail) {
    const detail = document.createElement("pre");
    detail.className = "log-detail";
    detail.textContent = entry.detail;
    detail.style.display = "none";
    div.style.cursor = "pointer";
    div.addEventListener("click", () => {
      detail.style.display = detail.style.display === "none" ? "block" : "none";
    });
    div.appendChild(detail);
  }

  return div;
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

export function mountLogPanel(root: HTMLElement): void {
  root.innerHTML = `
    <div class="log-header">
      <span class="log-title">Backend Activity</span>
      <span class="log-controls">
        <button id="log-clear" class="log-btn">Clear</button>
        <button id="log-scroll" class="log-btn log-btn-active">Auto-scroll</button>
      </span>
    </div>
    <div class="log-body" id="log-body"></div>
  `;

  logContainer = root.querySelector("#log-body");

  root.querySelector("#log-clear")?.addEventListener("click", () => {
    if (logContainer) logContainer.innerHTML = "";
  });

  const scrollBtn = root.querySelector("#log-scroll") as HTMLButtonElement;
  scrollBtn?.addEventListener("click", () => {
    autoScroll = !autoScroll;
    scrollBtn.classList.toggle("log-btn-active", autoScroll);
    scrollBtn.textContent = autoScroll ? "Auto-scroll" : "Scroll locked";
  });

  addSystemEntry("Connected to backend log stream");

  // Start polling
  pollLogs();
}

function addSystemEntry(msg: string): void {
  if (!logContainer) return;
  const div = document.createElement("div");
  div.className = "log-entry log-pipeline";
  div.innerHTML = `<span class="log-time">${formatTime()}</span>`
    + `<span class="log-cat" style="color:#ff7b72">[SYS]</span>`
    + `<span class="log-msg">${escapeHtml(msg)}</span>`;
  logContainer.appendChild(div);
}

function addLogEntry(entry: LogEntry): void {
  if (!logContainer) return;
  logContainer.appendChild(renderEntry(entry));
  if (autoScroll) {
    logContainer.scrollTop = logContainer.scrollHeight;
  }
}

async function pollLogs(): Promise<void> {
  while (true) {
    try {
      const resp = await fetch(`/api/logs?since=${lastSeq}`);
      if (resp.ok) {
        const data = await resp.json();
        const entries: LogEntry[] = data.entries || [];
        for (const entry of entries) {
          addLogEntry(entry);
        }
        if (data.seq) lastSeq = data.seq;
      }
    } catch {
      // Server down, keep polling
    }
    await new Promise(r => setTimeout(r, 500));
  }
}

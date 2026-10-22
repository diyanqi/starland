const HTML_ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
};

export function escapeHtml(value: unknown): string {
  return String(value ?? '').replace(/[&<>"']/g, (char) => HTML_ESCAPES[char] ?? char);
}

export function formatTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '';

  const diff = Date.now() - date.getTime();
  const minute = 60_000;
  const hour = 3_600_000;
  const day = 86_400_000;

  if (diff < minute) return '刚刚';
  if (diff < hour) return `${Math.floor(diff / minute)} 分钟前`;
  if (diff < day) return `${Math.floor(diff / hour)} 小时前`;
  if (diff < day * 7) return `${Math.floor(diff / day)} 天前`;

  return date.toLocaleDateString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  });
}

export function errorMessage(error: unknown): string {
  if (error instanceof Error) return error.message;
  return '出了点小问题，请稍后再试';
}

let toastTimer: number | undefined;

export function toast(message: string, kind: 'ok' | 'error' = 'ok'): void {
  let el = document.querySelector<HTMLDivElement>('.toast');
  if (!el) {
    el = document.createElement('div');
    el.className = 'toast';
    el.setAttribute('role', 'status');
    el.setAttribute('aria-live', 'polite');
    document.body.appendChild(el);
  }

  el.textContent = message;
  el.dataset.kind = kind;
  // 强制回流，保证连续弹出时也有动画。
  void el.offsetWidth;
  el.dataset.visible = 'true';

  window.clearTimeout(toastTimer);
  toastTimer = window.setTimeout(() => {
    if (el) el.dataset.visible = 'false';
  }, 3200);
}

export function debounce<T extends (...args: never[]) => void>(fn: T, wait = 300): T {
  let timer: number | undefined;
  return ((...args: Parameters<T>) => {
    window.clearTimeout(timer);
    timer = window.setTimeout(() => fn(...args), wait);
  }) as T;
}

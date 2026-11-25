<script lang="ts" module>
  // Стек открытых окон: Escape и Tab обрабатывает только верхнее.
  const stack: symbol[] = [];
</script>

<script lang="ts">
  import { onMount, type Snippet } from 'svelte';

  interface Props {
    id?: string;
    labelledby?: string;
    label?: string;
    /** Без обработчика окно не закрывается ни Escape, ни кликом по подложке. */
    onClose?: () => void;
    overlayClass?: string;
    class?: string;
    children: Snippet;
  }

  const {
    id,
    labelledby,
    label,
    onClose,
    overlayClass = 'bg-black/40 p-4',
    class: panelClass = '',
    children
  }: Props = $props();

  const token = Symbol('modal');
  let panel: HTMLDivElement;
  // Закрываем по клику на подложку, только если и нажатие началось на ней:
  // иначе выделение текста, отпущенное за краем окна, закрывало бы его.
  let pressedOnOverlay = false;

  const FOCUSABLE =
    'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

  function focusables(): HTMLElement[] {
    return Array.from(panel.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
      (el) => el.getClientRects().length > 0
    );
  }

  function handleKeydown(e: KeyboardEvent) {
    if (stack[stack.length - 1] !== token) return;

    if (e.key === 'Escape') {
      e.preventDefault();
      onClose?.();
      return;
    }
    if (e.key !== 'Tab') return;

    // Удерживаем фокус внутри окна: Tab с последнего элемента уходит на первый и наоборот.
    const items = focusables();
    const first = items[0];
    const last = items[items.length - 1];
    const active = document.activeElement;
    if (!first || !last) {
      e.preventDefault();
      panel.focus();
    } else if (e.shiftKey && (active === first || active === panel || !panel.contains(active))) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && (active === last || !panel.contains(active))) {
      e.preventDefault();
      first.focus();
    }
  }

  onMount(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    stack.push(token);
    panel.focus();
    return () => {
      stack.splice(stack.indexOf(token), 1);
      previous?.focus();
    };
  });
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- Клавиатура обрабатывается на уровне окна (Escape), клик по подложке — дублирующий способ закрыть. -->
<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div
  class="fixed inset-0 backdrop-blur-xs flex items-center justify-center z-50 overflow-hidden overscroll-contain {overlayClass}"
  onmousedown={(e) => (pressedOnOverlay = e.target === e.currentTarget)}
  onclick={(e) => {
    if (pressedOnOverlay && e.target === e.currentTarget) onClose?.();
    pressedOnOverlay = false;
  }}
>
  <div
    bind:this={panel}
    {id}
    role="dialog"
    aria-modal="true"
    aria-labelledby={labelledby}
    aria-label={label}
    tabindex="-1"
    class="bg-white border border-[#E8E7E3] rounded-lg shadow-2xl w-full flex flex-col outline-none animate-surface-enter {panelClass}"
  >
    {@render children()}
  </div>
</div>

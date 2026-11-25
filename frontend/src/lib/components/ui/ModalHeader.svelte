<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';

  interface Props {
    titleId: string;
    title: string;
    /** Без обработчика крестик не показывается. */
    onClose?: () => void;
    /** Перед заголовком: значок состояния. */
    leading?: Snippet;
    /** Сразу после заголовка: счётчик и т. п. */
    badge?: Snippet;
    /** Справа, перед крестиком: дополнительные кнопки. */
    actions?: Snippet;
  }

  const { titleId, title, onClose, leading, badge, actions }: Props = $props();
</script>

<div class="flex items-center justify-between gap-3 border-b border-[#F0EFEB] pb-3">
  <div class="flex items-center gap-2.5 min-w-0">
    {@render leading?.()}
    <h2 id={titleId} class="text-sm font-bold text-[#1C1917]">{title}</h2>
    {@render badge?.()}
  </div>
  <div class="flex items-center gap-1.5 shrink-0">
    {@render actions?.()}
    {#if onClose}
      <button
        type="button"
        onclick={onClose}
        class="h-7 w-7 flex items-center justify-center text-stone-400 hover:text-stone-700 hover:bg-stone-100 rounded-md transition pressable cursor-pointer"
        title="Закрыть"
        aria-label="Закрыть"
      >
        <Icon name="close" />
      </button>
    {/if}
  </div>
</div>

<script lang="ts">
  import Icon from '../ui/Icon.svelte';
  import { appState } from '../../stores/appState.svelte';

  import type { IconName } from '../ui/Icon.svelte';

  const typeStyles: Record<string, { bg: string; icon: IconName; iconClass: string; strokeWidth: number }> = {
    success: { bg: 'bg-[#ECFDF5] border-[#A7F3D0] text-[#065F46]', icon: 'check', iconClass: 'text-emerald-600', strokeWidth: 2.5 },
    error: { bg: 'bg-[#FEF2F2] border-[#FECACA] text-[#991B1B]', icon: 'close', iconClass: 'text-rose-600', strokeWidth: 2.5 },
    warning: { bg: 'bg-[#FFFBEB] border-[#FDE68A] text-[#92400E]', icon: 'warning', iconClass: 'text-amber-600', strokeWidth: 2 },
    info: { bg: 'bg-[#EFF6FF] border-[#BFDBFE] text-[#1E40AF]', icon: 'info', iconClass: 'text-blue-600', strokeWidth: 2 }
  };

  const currentStyle = $derived.by(() => {
    return (appState.toast && typeStyles[appState.toast.type]) || typeStyles.info!;
  });
</script>

{#if appState.toast}
  <div
    class="fixed bottom-6 right-6 z-50 max-w-md w-full shadow-xl transition-all"
    role="alert"
  >
    <div class="flex items-center gap-3 p-3.5 border rounded-lg text-xs font-mono animate-surface-enter {currentStyle.bg}">
      <span class="shrink-0 flex items-center justify-center w-4 h-4">
        <Icon name={currentStyle.icon} class="w-4 h-4 shrink-0 {currentStyle.iconClass}" strokeWidth={currentStyle.strokeWidth} />
      </span>
      <div class="flex-1 font-sans text-xs break-words">
        {appState.toast.message}
      </div>
      <button
        type="button"
        onclick={() => appState.hideToast()}
        class="text-stone-400 hover:text-stone-700 transition cursor-pointer shrink-0 flex items-center justify-center w-5 h-5 rounded hover:bg-black/5 pressable"
        aria-label="Закрыть уведомление"
      >
        <Icon name="close" class="w-3.5 h-3.5 shrink-0" />
      </button>
    </div>
  </div>
{/if}

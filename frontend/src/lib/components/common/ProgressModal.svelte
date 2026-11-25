<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { CancelCurrentOperation } from '../../api/wails';
  import Modal from '../ui/Modal.svelte';
  import ModalHeader from '../ui/ModalHeader.svelte';

  let isCanceling = $state(false);

  const modalTitle = $derived.by(() => {
    if (appState.progress.title) return appState.progress.title;
    return appState.currentAppMode === 'restore'
      ? 'Восстановление исходных данных'
      : 'Деперсонализация ПДн';
  });

  const statusText = $derived.by(() => {
    if (isCanceling) {
      return 'Отмена операции...';
    }
    return appState.currentAppMode === 'restore'
      ? 'Расшифровка ключа и восстановление данных...'
      : 'Потоковая обработка строк...';
  });

  const countText = $derived.by(() => {
    if (isCanceling) {
      return 'Остановка...';
    }
    if (appState.progress.total > 0) {
      return `${appState.progress.processed.toLocaleString('ru-RU')} / ${appState.progress.total.toLocaleString('ru-RU')} строк`;
    }
    if (appState.progress.processed > 0) {
      return `${appState.progress.processed.toLocaleString('ru-RU')} строк`;
    }
    return 'Инициализация...';
  });

  async function handleCancel() {
    if (isCanceling) return;
    isCanceling = true;
    try {
      await CancelCurrentOperation();
      setTimeout(() => {
        if (appState.showProgressModal && isCanceling) {
          appState.finishProgress();
          isCanceling = false;
        }
      }, 5000);
    } catch (err) {
      console.error('Failed to cancel operation:', err);
      appState.showToast('Ошибка при отмене операции', 'error');
      isCanceling = false;
    }
  }

  $effect(() => {
    if (!appState.showProgressModal) {
      isCanceling = false;
    }
  });
</script>

{#if appState.showProgressModal}
  <!-- Без onClose: окно прогресса закрывается только по завершении или отмене операции. -->
  <Modal id="progress-modal" labelledby="progress-modal-title" class="max-w-md p-6 gap-4">
      <ModalHeader titleId="progress-modal-title" title={modalTitle}>
        {#snippet leading()}
          <span id="progress-indicator" class="w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></span>
        {/snippet}
        {#snippet actions()}
          <span id="progress-pct-badge" class="font-mono text-xs font-bold text-[#1C1917] bg-[#F0EFEB] px-2 py-0.5 rounded border border-[#E8E7E3]">
            {appState.progress.percent}%
          </span>
        {/snippet}
      </ModalHeader>

      <div class="space-y-2">
        <div class="w-full bg-[#F0EFEB] h-2.5 rounded-full overflow-hidden p-0.5 border border-[#E8E7E3]">
          <div
            id="progress-bar-fill"
            class="progress-fill bg-[#1C1917] h-full rounded-full"
            style="width: {appState.progress.percent}%"
          ></div>
        </div>
        <div class="flex items-center justify-between text-xs font-mono text-[#78716C]">
          <span id="progress-status-text">{statusText}</span>
          <span id="progress-count-text">{countText}</span>
        </div>
      </div>

      <div class="flex items-center justify-end space-x-2 pt-2 border-t border-[#F0EFEB]">
        <button
          id="btn-cancel-progress"
          type="button"
          onclick={handleCancel}
          disabled={isCanceling}
          class="px-4 py-2 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold rounded-md pressable disabled:opacity-50 cursor-pointer"
        >
          {isCanceling ? 'Отмена...' : 'Отмена'}
        </button>
      </div>
  </Modal>
{/if}

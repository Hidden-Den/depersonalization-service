<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { OpenContainingFolder } from '../../api/wails';
  import { copyToClipboard } from '../../services/clipboard';
  import { splitPath } from '../../utils/path';

  import Modal from '../ui/Modal.svelte';
  import ModalHeader from '../ui/ModalHeader.svelte';
  import Icon, { type IconName } from '../ui/Icon.svelte';

  type SecretKind = 'archive' | 'key';
  let copiedKind = $state<SecretKind | null>(null);

  const getDirectory = (filePath: string) => splitPath(filePath).dir;
  const getFileName = (filePath: string) => splitPath(filePath).filename;

  function formatDuration(ms: number): string {
    if (ms < 1000) {
      return `${ms} мс`;
    }
    const totalSeconds = Math.round(ms / 1000);
    if (totalSeconds < 60) {
      return `${totalSeconds} сек`;
    }
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    if (seconds === 0) {
      return `${minutes} мин`;
    }
    return `${minutes} мин ${seconds} сек`;
  }

  const isRestore = $derived.by(() => {
    return (
      appState.currentAppMode === 'restore' ||
      (appState.successResult !== null && 'restoredCells' in appState.successResult)
    );
  });

  const modalTitle = $derived(
    isRestore ? 'Восстановление успешно завершено' : 'Деперсонализация успешно завершена'
  );


  const savedFiles = $derived.by(() => {
    const res = appState.successResult;
    if (!res) return [];
    const files: { path: string; icon: IconName; iconClass: string; title: string }[] = [];
    if (res.outputPath) {
      files.push({ path: res.outputPath, icon: 'document', iconClass: 'text-emerald-700', title: 'Показать файл в папке' });
    }
    if (!isRestore && res.archivePath) {
      files.push({ path: res.archivePath, icon: 'archive', iconClass: 'text-amber-600', title: 'Показать архив в папке' });
    }
    if (!isRestore && res.keyPath) {
      files.push({ path: res.keyPath, icon: 'key', iconClass: 'text-stone-500', title: 'Показать ключ в папке' });
    }
    return files;
  });

  const secrets = $derived.by(() => {
    const res = appState.successResult;
    if (!res || isRestore) return [];
    const list: { kind: SecretKind; label: string; value: string; hint?: string }[] = [];
    if (res.archivePassword) {
      list.push({ kind: 'archive', label: 'Пароль архива', value: res.archivePassword });
    }
    if ('keyPasswordGenerated' in res && res.keyPasswordGenerated && res.effectivePassword) {
      list.push({
        kind: 'key',
        label: 'Пароль ключа',
        value: res.effectivePassword,
        hint: 'Сохраните пароль и файл-ключ в безопасном месте для восстановления данных.'
      });
    }
    return list;
  });

  async function copySecret(kind: SecretKind, label: string, value: string) {
    if (await copyToClipboard(value)) {
      copiedKind = kind;
      setTimeout(() => {
        if (copiedKind === kind) copiedKind = null;
      }, 2000);
      appState.showToast(`${label} скопирован в буфер обмена!`, 'success');
    } else {
      appState.showToast(`Не удалось скопировать ${label.toLowerCase()}`, 'error');
    }
  }

  const formattedDuration = $derived.by(() => {
    return formatDuration(appState.successResult?.durationMs || 0);
  });

  const statsCount = $derived.by(() => {
    const res = appState.successResult;
    if (!res) return null;
    if ('restoredCells' in res && typeof res.restoredCells === 'number') {
      return {
        label: 'Восстановлено ячеек с ПДн',
        value: res.restoredCells.toLocaleString('ru-RU')
      };
    }
    if ('rowsCount' in res && typeof res.rowsCount === 'number') {
      return {
        label: 'Обработано строк',
        value: res.rowsCount.toLocaleString('ru-RU')
      };
    }
    return null;
  });

  async function handleRevealFile(targetPath?: string) {
    if (!targetPath) return;
    try {
      await OpenContainingFolder(targetPath);
    } catch (err) {
      console.error('Failed to open containing folder:', err);
      appState.showToast('Не удалось открыть папку', 'error');
    }
  }

  async function handleRevealFolder() {
    const path = appState.successResult?.outputPath || appState.successResult?.archivePath || appState.successResult?.keyPath;
    await handleRevealFile(path);
  }
</script>

{#if appState.showSuccessModal && appState.successResult}
  <Modal
    id="success-modal"
    labelledby="success-modal-title"
    onClose={() => appState.closeSuccessModal()}
    class="max-w-lg p-6 gap-4"
  >
      <ModalHeader titleId="success-modal-title" title={modalTitle} onClose={() => appState.closeSuccessModal()}>
        {#snippet leading()}
          <div class="w-6 h-6 rounded-full bg-emerald-50 border border-emerald-200/80 flex items-center justify-center text-emerald-600">
            <Icon name="check" class="w-3.5 h-3.5" strokeWidth={2.5} />
          </div>
        {/snippet}
      </ModalHeader>

      <div class="border border-[#E8E7E3] bg-[#FAF9F5] rounded-lg p-3 space-y-2">
        <div class="text-[11px] font-semibold text-[#78716C] uppercase tracking-wider">
          Сохраненные файлы
        </div>

        <div class="space-y-1.5">
          {#each savedFiles as file (file.path)}
            <div class="flex items-center justify-between gap-2 bg-white border border-[#E8E7E3] rounded-md p-2">
              <div class="min-w-0 flex items-center gap-2">
                <Icon name={file.icon} class="w-3.5 h-3.5 shrink-0 {file.iconClass}" strokeWidth={1.8} />
                <div class="min-w-0">
                  <div class="text-xs font-mono font-medium text-[#1C1917] truncate select-all" title={file.path}>
                    {getFileName(file.path)}
                  </div>
                  <div class="text-[10px] text-[#78716C] font-mono truncate select-all" title={file.path}>
                    {getDirectory(file.path)}
                  </div>
                </div>
              </div>
              <button
                type="button"
                onclick={() => handleRevealFile(file.path)}
                class="px-2 py-1 bg-[#FAF9F5] hover:bg-[#F5F5F4] border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-[11px] font-medium rounded shadow-2xs pressable shrink-0 cursor-pointer"
                title={file.title}
              >
                Показать
              </button>
            </div>
          {/each}
        </div>
      </div>

      {#each secrets as secret (secret.kind)}
        <div class="border border-[#E8E7E3] bg-[#FAF9F5] p-3.5 rounded-lg space-y-2">
          <div class="flex items-center justify-between gap-2">
            <span class="text-xs font-semibold text-[#1C1917]">{secret.label}</span>
            <button
              id={secret.kind === 'key' ? 'copy-key-password' : undefined}
              type="button"
              onclick={() => copySecret(secret.kind, secret.label, secret.value)}
              class="px-2.5 py-1 bg-white hover:bg-[#F5F5F4] border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-medium rounded-md shadow-2xs pressable flex items-center gap-1.5 cursor-pointer"
            >
              {#if copiedKind === secret.kind}
                <Icon name="check" class="w-3 h-3 text-emerald-600" strokeWidth={2.5} />
                <span class="text-emerald-700 font-semibold">Скопировано</span>
              {:else}
                <Icon name="copy" class="w-3 h-3 text-stone-500" />
                <span>Скопировать</span>
              {/if}
            </button>
          </div>
          <div class="text-xs font-mono font-semibold text-[#1C1917] select-all bg-white px-3 py-2 border border-[#E8E7E3] rounded-md tracking-wider">
            <code>{secret.value}</code>
          </div>
          {#if secret.hint}
            <div class="text-[11px] text-[#78716C] leading-normal font-sans">{secret.hint}</div>
          {/if}
        </div>
      {/each}

      {#if statsCount || formattedDuration}
        <div class="flex items-center justify-between text-xs text-[#78716C] pt-0.5 font-sans">
          <div>
            {#if statsCount}
              <span>{statsCount.label}: </span>
              <span class="font-semibold text-[#1C1917] font-mono">{statsCount.value}</span>
            {/if}
          </div>
          <div class="shrink-0">
            <span>Время: </span>
            <span class="font-semibold text-[#1C1917] font-mono">{formattedDuration}</span>
          </div>
        </div>
      {/if}

      <div class="flex items-center justify-end space-x-2 pt-3 border-t border-[#F0EFEB]">
        {#if appState.successResult.outputPath || appState.successResult.archivePath}
          <button
            id="btn-reveal-folder"
            type="button"
            onclick={handleRevealFolder}
            class="px-3.5 py-1.5 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold rounded-md transition cursor-pointer flex items-center gap-1.5 shadow-2xs pressable"
          >
            <Icon name="folder" class="w-3.5 h-3.5 text-stone-600" />
            <span>Показать в папке</span>
          </button>
        {/if}
        <button
          id="btn-close-success"
          type="button"
          onclick={() => appState.closeSuccessModal()}
          class="px-4 py-1.5 bg-[#1C1917] hover:bg-[#292524] text-white text-xs font-semibold rounded-md transition cursor-pointer shadow-sm pressable"
        >
          Закрыть
        </button>
      </div>
  </Modal>
{/if}

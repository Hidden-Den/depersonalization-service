<script lang="ts">
  import { restoreState } from '../../stores/restoreState.svelte';
  import { appState } from '../../stores/appState.svelte';
  import { SaveFileDialog, RestoreFileTo } from '../../api/wails';
  import { getDefaultRestoredPath, splitPath } from '../../utils/path';
  import Icon, { type IconName } from '../ui/Icon.svelte';
  import PasswordInput from '../ui/PasswordInput.svelte';
  import type { Snippet } from 'svelte';

  interface FileSlot {
    title: string;
    status: Snippet;
    icon: IconName;
    path: string;
    name: string;
    loading?: boolean;
    dropzoneId: string;
    infoId: string;
    nameId: string;
    emptyTitle: string;
    emptyHint: string;
    selectLabel: string;
    onSelect: () => void;
    onLoad: (path: string) => void;
    onClear: () => void;
    changeId?: string;
    removeId: string;
  }

  // Путь берётся из File.path, который есть только в desktop-webview.
  function droppedPath(e: DragEvent): string | null {
    const file = e.dataTransfer?.files?.[0] as (File & { path?: string }) | undefined;
    const path = file?.path;
    return path && (path.includes('/') || path.includes('\\')) ? path : null;
  }

  const dataFileName = $derived(splitPath(restoreState.dataPath).filename || 'data.xlsx');
  const keyFileName = $derived(splitPath(restoreState.keyPath).filename || 'key.depmap');

  const isReady = $derived(
    !!(restoreState.dataPath && restoreState.keyInfo && restoreState.keyInfo.valid)
  );

  let isRestoring = $state(false);

  const isPasswordError = $derived(
    !!(restoreState.keyPath && restoreState.keyInfo && !restoreState.keyInfo.valid && restoreState.password)
  );

  const isKeyValid = $derived(
    !!(restoreState.keyInfo && restoreState.keyInfo.valid)
  );

  const statusInfo = $derived.by(() => {
    if (isReady) {
      return {
        dotClass: 'w-2 h-2 rounded-full bg-emerald-600 animate-pulse',
        text: 'Готово к восстановлению',
        textClass: 'text-emerald-800 font-medium'
      };
    }
    if (!restoreState.dataPath && !restoreState.keyPath) {
      return {
        dotClass: 'w-2 h-2 rounded-full bg-stone-400',
        text: 'Ожидание файла данных',
        textClass: 'text-stone-600'
      };
    }
    if (!restoreState.dataPath) {
      return {
        dotClass: 'w-2 h-2 rounded-full bg-amber-500',
        text: 'Ожидание файла данных',
        textClass: 'text-stone-600'
      };
    }
    if (!restoreState.keyPath) {
      return {
        dotClass: 'w-2 h-2 rounded-full bg-amber-500',
        text: 'Ожидание ключа .depmap',
        textClass: 'text-stone-600'
      };
    }
    if (restoreState.keyInfo && !restoreState.keyInfo.valid) {
      if (!restoreState.password) {
        return {
          dotClass: 'w-2 h-2 rounded-full bg-amber-500',
          text: 'Требуется пароль ключа',
          textClass: 'text-stone-600'
        };
      }
      return {
        dotClass: 'w-2 h-2 rounded-full bg-rose-500',
        text: 'Неверный пароль ключа',
        textClass: 'text-rose-700 font-medium'
      };
    }
    return {
      dotClass: 'w-2 h-2 rounded-full bg-stone-400',
      text: 'Ожидание файла данных',
      textClass: 'text-stone-600'
    };
  });

  async function executeRestore() {
    if (isRestoring || appState.isProcessing) return;
    if (
      !restoreState.dataPath ||
      !restoreState.keyPath ||
      !restoreState.keyInfo ||
      !restoreState.keyInfo.valid
    ) {
      appState.showToast(
        'Пожалуйста, укажите файл с данными и валидный ключ восстановления (.depmap)',
        'warning'
      );
      return;
    }

    const ext = restoreState.dataPath.split('.').pop() || 'xlsx';
    const defaultPath = getDefaultRestoredPath(restoreState.dataPath, ext);

    try {
      const destPath = await SaveFileDialog(defaultPath);
      if (!destPath) {
        // User cancelled dialog
        return;
      }

      isRestoring = true;

      let totalRows = restoreState.dataInfo?.rowCount || 0;
      if (restoreState.dataInfo?.sheets && restoreState.dataInfo.sheets.length > 1) {
        totalRows = restoreState.dataInfo.sheets.reduce(
          (acc, s) => acc + (s.rowCount || 0),
          0
        );
      }

      appState.startProgress('Восстановление данных...');
      if (totalRows > 0) {
        appState.updateProgress(0, 0, totalRows);
      }

      const res = await RestoreFileTo(
        restoreState.dataPath,
        restoreState.keyPath,
        restoreState.password,
        destPath
      );

      if (res && res.success) {
        appState.showSuccess(res);
      } else {
        const errMsg = res?.error || 'Неизвестная ошибка восстановления';
        appState.showToast(`Ошибка при восстановлении данных: ${errMsg}`, 'error');
      }
    } catch (err: any) {
      console.error('Failed to restore file:', err);
      const errStr = String(err);
      if (errStr.includes('context canceled') || errStr.includes('canceled') || errStr.includes('отменена')) {
        appState.showToast('Операция отменена', 'info');
      } else {
        appState.showToast(`Ошибка восстановления: ${err}`, 'error');
      }
    } finally {
      isRestoring = false;
      appState.finishProgress();
    }
  }
</script>

<div id="restoration-view" class="flex flex-col gap-6 pb-12">
  <!-- Header Banner -->
  <div class="flex flex-col md:flex-row md:items-center justify-between gap-4">
    <div>
      <h2 class="text-2xl font-bold tracking-tight text-[#1C1917]">
        Восстановление исходных данных
      </h2>
      <p class="text-xs text-[#78716C] mt-1 font-sans">
        Деобезличивание документов с использованием зашифрованного ключа .depmap
      </p>
    </div>

    <!-- Live Step Status -->
    <div
      id="restore-hero-status"
      class="flex items-center space-x-2 text-xs font-sans px-3.5 py-1.5 bg-white border border-[#E8E7E3] rounded-md shadow-2xs self-start md:self-auto"
    >
      <span id="restore-status-dot" class={statusInfo.dotClass}></span>
      <span id="restore-status-text" class={statusInfo.textClass}>{statusInfo.text}</span>
    </div>
  </div>

  {#snippet pill(text: string, color: 'emerald' | 'amber', pulse = false)}
    <span class="text-xs font-medium flex items-center gap-1.5 {color === 'emerald' ? 'text-emerald-700' : 'text-amber-600'}">
      <span class="w-1.5 h-1.5 rounded-full {color === 'emerald' ? 'bg-emerald-600' : 'bg-amber-500'} {pulse ? 'animate-pulse' : ''}"></span>
      {text}
    </span>
  {/snippet}

  {#snippet dataStatus()}
    {#if restoreState.dataPath}
      {@render pill('Готов', 'emerald')}
    {:else}
      <span class="text-[10px] font-mono text-[#A8A29E]">Таблица / Текст</span>
    {/if}
  {/snippet}

  {#snippet keyStatus()}
    {#if !restoreState.keyPath}
      <span class="text-[10px] bg-stone-100 border border-[#E8E7E3] px-2 py-0.5 rounded whitespace-nowrap text-[#78716C] font-mono">.depmap</span>
    {:else if restoreState.isInspectingKey}
      {@render pill('Проверка...', 'amber', true)}
    {:else if isKeyValid}
      {@render pill('Готов', 'emerald')}
    {:else}
      {@render pill('Зашифрован', 'amber')}
    {/if}
  {/snippet}

  {#snippet fileCard(slot: FileSlot)}
    <div class="bg-white border border-[#E8E7E3] rounded-lg p-5 shadow-xs flex flex-col justify-between min-h-[260px]">
      <div>
        <div class="flex items-center justify-between text-xs text-[#78716C] mb-3 h-6 shrink-0">
          <span class="font-semibold text-[#1C1917] tracking-tight">{slot.title}</span>
          {@render slot.status()}
        </div>

        {#if slot.loading}
          <div class="flex flex-col items-center justify-center p-6 text-center bg-[#FAF9F5] border border-[#E8E7E3] rounded-lg gap-2.5 select-none hover-sheen">
            <div class="w-6 h-6 border-2 border-[#E8E7E3] border-t-[#1C1917] rounded-full animate-spin"></div>
            <div class="font-semibold text-xs text-[#1C1917]">Чтение структуры...</div>
            <div class="text-[11px] text-[#78716C]">Определение строк и колонок</div>
          </div>
        {:else if !slot.path}
          <div
            id={slot.dropzoneId}
            class="flex flex-col items-center justify-center p-6 text-center border border-dashed border-[#D6D3D1] hover:border-[#1C1917] bg-[#FAF9F5] hover:bg-stone-50 transition cursor-pointer select-none group rounded-lg hover-sheen"
            role="button"
            tabindex="0"
            onclick={slot.onSelect}
            onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') slot.onSelect(); }}
            ondragover={(e) => { e.preventDefault(); if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy'; }}
            ondrop={(e) => {
              e.preventDefault();
              const path = droppedPath(e);
              if (path) slot.onLoad(path);
            }}
          >
            <Icon name={slot.icon} class="w-8 h-8 text-stone-400 group-hover:text-stone-700 transition-colors mb-2" strokeWidth={1.5} />
            <div class="font-semibold text-xs text-[#1C1917]">{slot.emptyTitle}</div>
            <div class="text-[11px] text-[#A8A29E] mt-0.5 mb-3.5">{slot.emptyHint}</div>
            <button
              type="button"
              onclick={(e) => {
                e.stopPropagation();
                slot.onSelect();
              }}
              class="px-3.5 py-1.5 bg-white border border-[#E8E7E3] group-hover:border-[#1C1917] text-xs font-semibold text-[#1C1917] rounded-md shadow-2xs pressable cursor-pointer"
            >
              {slot.selectLabel}
            </button>
          </div>
        {:else}
          <div id={slot.infoId} class="py-7 flex items-center gap-3">
            <div class="w-9 h-9 rounded-md bg-[#FAF9F5] border border-[#E8E7E3] flex items-center justify-center shrink-0 text-emerald-700">
              <Icon name={slot.icon} strokeWidth={1.8} />
            </div>
            <div class="min-w-0 flex-1">
              <div class="font-bold text-xs text-[#1C1917] truncate" id={slot.nameId} title={slot.name}>
                {slot.name}
              </div>
            </div>
          </div>
        {/if}
      </div>

      {#if slot.path}
        <div class="pt-4 border-t border-[#F0EFEB] flex items-center justify-end gap-2">
          <button
            type="button"
            id={slot.changeId}
            onclick={slot.onSelect}
            class="h-7 px-3 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold rounded-md shadow-2xs pressable flex items-center justify-center cursor-pointer"
          >
            Сменить
          </button>
          <button
            type="button"
            id={slot.removeId}
            onclick={slot.onClear}
            class="h-7 px-3 bg-white border border-[#E8E7E3] hover:border-rose-300 hover:text-rose-600 text-[#78716C] text-xs font-semibold rounded-md shadow-2xs pressable flex items-center justify-center cursor-pointer"
          >
            Удалить
          </button>
        </div>
      {/if}
    </div>
  {/snippet}

  <div class="grid grid-cols-1 lg:grid-cols-3 gap-5 items-stretch">
    {@render fileCard({
      title: 'Файл данных',
      status: dataStatus,
      icon: 'document',
      path: restoreState.dataPath,
      name: dataFileName,
      loading: restoreState.isLoadingData,
      dropzoneId: 'restore-data-dropzone',
      infoId: 'restore-data-info',
      nameId: 'restore-data-name',
      emptyTitle: 'Выберите или перетащите файл',
      emptyHint: 'XLSX, ODS, CSV, DOCX, ODT',
      selectLabel: 'Выбрать файл',
      onSelect: () => restoreState.selectDataFile(),
      onLoad: (path) => restoreState.loadDataFile(path),
      onClear: () => restoreState.clearDataFile(),
      removeId: 'btn-remove-data-file'
    })}

    {@render fileCard({
      title: 'Ключ деобезличивания',
      status: keyStatus,
      icon: 'key',
      path: restoreState.keyPath,
      name: keyFileName,
      dropzoneId: 'restore-key-dropzone',
      infoId: 'restore-key-info',
      nameId: 'restore-key-name',
      emptyTitle: 'Выберите ключ восстановления',
      emptyHint: 'Файл с расширением .depmap',
      selectLabel: 'Выбрать ключ',
      onSelect: () => restoreState.selectKeyFile(),
      onLoad: (path) => restoreState.loadKeyFile(path),
      onClear: () => restoreState.clearKeyFile(),
      changeId: 'btn-reselect-key',
      removeId: 'btn-remove-key-file'
    })}

    <!-- Пароль и запуск -->
    <div class="bg-white border border-[#E8E7E3] rounded-lg p-5 shadow-xs flex flex-col justify-between min-h-[260px]">
      <div>
        <div class="flex items-center justify-between text-xs text-[#78716C] mb-3 h-6 shrink-0">
          <span class="font-semibold text-[#1C1917] tracking-tight">Пароль ключа</span>
          <span class="text-[10px] bg-stone-100 border border-[#E8E7E3] px-2 py-0.5 rounded whitespace-nowrap text-[#78716C] font-mono">AES-256</span>
        </div>

        <div class="space-y-2">
          <PasswordInput
            id="restore-key-password"
            bind:value={() => restoreState.password, (v) => restoreState.setPassword(v)}
            placeholder="Пароль защиты ключа"
            class="py-2 focus:bg-white transition"
            onkeydown={async (e) => {
              if (e.key === 'Enter') {
                await restoreState.inspectKeyNow();
                if (isReady) executeRestore();
              }
            }}
          />
          {#if isPasswordError}
            <p class="text-[11px] text-rose-600 font-sans mt-1.5 flex items-center gap-1">
              <span>Неверный пароль ключа</span>
            </p>
          {:else if isKeyValid}
            <p class="text-[11px] text-emerald-700 font-sans mt-1.5 flex items-center gap-1">
              <span>Ключ готов к восстановлению</span>
            </p>
          {:else}
            <p class="text-[11px] text-[#A8A29E] mt-1.5 font-sans">
              {restoreState.keyPath ? 'Введите пароль для расшифровки' : 'Сначала укажите файл и ключ'}
            </p>
          {/if}
        </div>
      </div>

      <div class="pt-4 border-t border-[#E8E7E3]">
        <button
          id="btn-restore-start"
          type="button"
          onclick={executeRestore}
          disabled={!isReady || isRestoring || appState.isProcessing}
          class="w-full bg-[#1C1917] disabled:bg-stone-200 disabled:text-stone-400 disabled:cursor-not-allowed hover:bg-[#292524] text-white text-xs font-semibold py-2.5 rounded-md shadow-xs pressable flex items-center justify-center gap-1.5 cursor-pointer font-sans"
        >
          <span>Восстановить данные</span>
        </button>
      </div>
    </div>
  </div>
</div>
